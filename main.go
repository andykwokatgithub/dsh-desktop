// dsh-desktop is a Go + WebView2 desktop shell for the DeepSeek Harness Web UI.
//
// It identifies the dsh web backend (whose instance is it? is it dsh at all?),
// starts or reuses one, enforces a single running instance, and embeds the UI in
// a native window. Every startup interaction -- loading, errors, the token input
// for somebody else's instance -- is a local HTML page inside that window.
//
// Parameter errors are the one exception, and they go the other way: a bad
// command line is a console concern (whoever typed it is looking at a terminal),
// so it is reported on stderr and never in a native dialog. The remaining native
// dialogs are reserved for pre-window failures that are not command-line
// mistakes (single instance, window creation).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	webviewlib "github.com/webview/webview_go"
	"golang.org/x/sys/windows"

	"github.com/deepseek-ai/dsh-desktop/internal/appdir"
	"github.com/deepseek-ai/dsh-desktop/internal/config"
	"github.com/deepseek-ai/dsh-desktop/internal/dpi"
	"github.com/deepseek-ai/dsh-desktop/internal/procinfo"
	"github.com/deepseek-ai/dsh-desktop/internal/service"
	"github.com/deepseek-ai/dsh-desktop/internal/singleinstance"
	"github.com/deepseek-ai/dsh-desktop/internal/ui"
	"github.com/deepseek-ai/dsh-desktop/internal/update"
	dswebview "github.com/deepseek-ai/dsh-desktop/internal/webview"
)

func main() {
	// Declare DPI awareness before any window is created. webview_go only calls
	// enable_dpi_awareness() when it owns the window (m_owns_window == true);
	// since we pass our own HWND, the library skips it entirely. Without this,
	// Windows bitmap-scales the whole window and WebView2 text renders blurry.
	dpi.SetAwareness()

	// webview_go requires the window and message loop on a locked OS thread.
	runtime.LockOSThread()

	// Flags come first, before the console is hidden: a bad parameter is a
	// command-line mistake and is reported on the console like any other CLI
	// error (see reportConfigError), so the console that the console-subsystem
	// build created must still be visible while the message is printed.
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, config.Usage()) // localized help, printed once
			os.Exit(0)
		}
		reportConfigError(err)
	}
	if cfg.ShowVersion {
		fmt.Printf("dsh-desktop %s\n", config.Version)
		os.Exit(0)
	}

	// Update path: handled as a plain CLI command before the GUI/single-instance
	// path, so --check-update / --update never open a window.
	if cfg.CheckUpdate || cfg.Update {
		runUpdateCmd(cfg)
		os.Exit(0)
	}

	// GUI launch from here on. The console-subsystem build gives the process a
	// console even on double-click, so swallow that window -- except when it is
	// shared with the terminal this was started from (see hideConsole).
	hideConsole()

	// FR-03: single instance. If another is running, activate it and exit.
	_, ok, err := singleinstance.Acquire()
	if err != nil {
		fatal(1, "单实例错误", err.Error())
	}
	if !ok {
		_, _ = singleinstance.ActivateExisting()
		os.Exit(0)
	}

	// FR-02: the window comes first. Its embedded HTML pages carry every startup
	// interaction, so the decision below can report progress, ask for a token, or
	// show an error without a native dialog. The title carries the running
	// version so a screenshot of the window identifies the build.
	view, win, err := singleinstance.NewWindow(singleinstance.WindowOptions{
		Title:    cfg.WindowTitleText(),
		Width:    cfg.WindowWidth,
		Height:   cfg.WindowHeight,
		DevTools: cfg.DevTools,
	})
	if err != nil {
		fatal(1, "窗口创建失败", err.Error())
	}

	a := &app{cfg: cfg, view: view, win: win, endpointPath: appdir.Path(appdir.EndpointFile)}
	view.Init(dswebview.SecurityInit(cfg.ContextMenu))
	_ = dswebview.BindExternal(view)
	_ = dswebview.BindAuthGuard(view, a.onAuthFence)
	_ = dswebview.BindTokenTarget(view, a.tokenTarget)
	_ = dswebview.BindTokenSubmit(view, a.onTokenSubmit)
	_ = dswebview.BindTokenSkip(view, a.onTokenSkip)
	_ = dswebview.BindRetry(view, a.onRetry)
	view.SetHtml(ui.Loading)

	// The decision runs off the UI thread; every UI effect hops back via Dispatch.
	go a.startup()

	// Block until WM_CLOSE triggers Terminate (see singleinstance.WndProc).
	view.Run()

	view.Destroy()
	a.shutdown()
	os.Exit(0)
}

// app owns the shell's runtime state: the instance it may have started, the
// endpoint it is showing, and the token page's target.
type app struct {
	cfg          *config.Config
	view         webviewlib.WebView
	win          *singleinstance.Window
	endpointPath string

	mu               sync.Mutex
	spawnerPID       int   // cmd.exe wrapper of the instance we own (0 = none)
	spawnerStartedAt int64 // its creation time, so every kill is proven safe
	owned            bool
	lastURL          string
	tokenHost        string
	tokenPort        int
	running          bool // a startup decision is in flight
	fenceHandled     bool
}

// behavior builds the probe/spawn parameters for one endpoint.
func (a *app) behavior(host string, port int) service.Behavior {
	return service.Behavior{
		Host:           host,
		Port:           port,
		Command:        a.cfg.Command,
		StartupTimeout: a.cfg.StartupTimeoutDuration(),
		PollInterval:   a.cfg.PollIntervalDuration(),
	}
}

func (a *app) logf(format string, args ...any) {
	_ = appendLog(fmt.Sprintf(format, args...))
}

// page replaces whatever is shown with a local HTML page (UI thread).
func (a *app) page(html string) {
	a.view.Dispatch(func() { a.view.SetHtml(html) })
}

// eval runs a script in the current page (UI thread).
func (a *app) eval(js string) {
	a.view.Dispatch(func() { a.view.Eval(js) })
}

// navigate shows url and remembers it as the page the local error page should
// return to.
func (a *app) navigate(url string) {
	a.mu.Lock()
	a.lastURL = url
	a.mu.Unlock()
	a.view.Dispatch(func() { a.view.Navigate(url) })
}

func (a *app) setOwned(spawnerPID int, spawnerStartedAt int64, owned bool) {
	a.mu.Lock()
	a.spawnerPID, a.spawnerStartedAt, a.owned = spawnerPID, spawnerStartedAt, owned
	a.mu.Unlock()
}

// startup decides what to show for the configured endpoint. It is the single
// place that answers "reuse, attach, ask for a token, or start our own", and it
// runs off the UI thread (service probing and spawning block).
func (a *app) startup() {
	cfg := a.cfg
	host, port := cfg.Host, cfg.Port
	behavior := a.behavior(host, port)

	record, hasRecord := service.LoadEndpoint(a.endpointPath)
	recordedOurs := hasRecord && record.Live(host, port)
	state := behavior.Probe()
	pid, occupant, evidence := identify(host, port)

	// 1) Our own recorded instance: reuse it through the persisted cookie.
	if recordedOurs {
		if state == service.StateHealthy {
			a.setOwned(record.SpawnerPID, record.SpawnerStartedAt, true)
			a.logf("复用自有实例 %s:%d（listenerPid=%d spawnerPid=%d）", host, port, record.ListenerPID, record.SpawnerPID)
			a.navigate(cfg.PageURL())
			return
		}
		// Ours but no longer serving: clean the tree up, then decide again. The
		// kill still has to be proven safe (see service.StopOwned).
		a.logf("清理自有陈旧实例 %s:%d（listenerPid=%d spawnerPid=%d）", host, port, record.ListenerPID, record.SpawnerPID)
		if killed, err := service.StopOwned(record, 0, 0); err != nil && !errors.Is(err, service.ErrNotOwned) {
			a.logf("清理自有陈旧实例失败: %v", err)
		} else if len(killed) > 0 {
			a.logf("已清理自有陈旧实例的进程树: %v", killed)
		}
		_ = service.ClearEndpoint(a.endpointPath)
	}

	// 2) An explicit --url token: attach when it still validates.
	if token := cfg.URLToken(); token != "" {
		ok, err := service.ValidateToken(context.Background(), host, port, token)
		if err == nil && ok {
			a.logf("附着 --url 指定的实例 %s:%d（token 有效）", host, port)
			a.navigate(cfg.PageURL())
			return
		}
		if err != nil {
			a.logf("--url 的 token 校验无法完成: %v", err)
		} else {
			a.logf("--url 的 token 无效，改为提示用户输入 token")
		}
		a.showTokenPage(host, port)
		return
	}

	// 3) Somebody else's dsh: try the persisted browser cookie first. A rejection
	//    lands on onAuthFence, which turns it into the token page -- so a working
	//    cookie keeps the fast path, and only "when authentication cannot be
	//    obtained" does the user get asked for a token (FR-05).
	if evidence.Strong {
		if state == service.StateHealthy {
			a.logf("端口上是非本壳 DSH 实例 %s:%d（pid=%d image=%q keywords=%v），先按 cookie 附着",
				host, port, pid, occupant.ImagePath, evidence.Keywords)
			a.navigate(cfg.CanonicalURL())
			return
		}
		a.logf("端口上是疑似启动中的非本壳 DSH 实例 %s:%d（pid=%d），显示 token 输入页", host, port, pid)
		a.showTokenPage(host, port)
		return
	}

	// 4) Anything else on the port: never touch it, start our own elsewhere.
	prefer := port
	if state != service.StateFree {
		a.logf("端口 %s:%d 被非 dsh 进程占用（pid=%d image=%q），改用回退端口", host, port, pid, occupant.ImagePath)
		prefer = 0
	}
	a.spawnOwn(prefer)
}

// spawnOwn starts an instance this shell owns: on prefer when that port is free,
// otherwise on a free port >= service.FallbackPortMin. The endpoint record it
// writes is what lets the next launch recognise the instance as ours.
func (a *app) spawnOwn(prefer int) {
	host := a.cfg.Host
	port := 0
	if prefer > 0 && a.behavior(host, prefer).Probe() == service.StateFree {
		port = prefer
	}
	if port == 0 {
		picked, err := service.PickFreePort(host, service.FallbackPortMin)
		if err != nil {
			a.showError("无法找到可用端口", err.Error())
			return
		}
		port = picked
	}

	behavior := a.behavior(host, port)
	child, err := service.Spawn(context.Background(), behavior)
	if err != nil {
		a.showError("无法启动 DeepSeek Harness", fmt.Sprintf("启动失败: %v\n请确认已全局安装 dsh 命令（npm i -g @deepseek-ai/dsh）且 node、dsh 均位于 PATH。", err))
		return
	}
	spawnerStartedAt := int64(0)
	if started, err := procinfo.StartTime(child.PID()); err == nil {
		spawnerStartedAt = started.UnixNano()
	}
	// Claim the wrapper the moment it exists, not once the service is ready: a
	// window closed while the service is still starting must still be able to
	// stop the instance this shell just created (--stop-on-exit).
	a.setOwned(child.PID(), spawnerStartedAt, true)

	readyURL, err := service.WaitHealthy(context.Background(), behavior, child)
	if err != nil {
		// The child is our direct child, which is itself the proof StopOwned
		// needs; nothing unrelated can be hit even if the PID is gone already.
		// It is gone after this, so release the claim again.
		_, _ = service.StopOwned(service.Endpoint{}, child.PID(), spawnerStartedAt)
		a.setOwned(0, 0, false)
		a.showError("服务启动失败", fmt.Sprintf("%s:%d 未在超时时间内就绪: %v", host, port, err))
		return
	}

	listenerPID, startedAt := service.ListenerMemory(host, port)
	record := service.Endpoint{
		Host:              host,
		Port:              port,
		ListenerPID:       listenerPID,
		ListenerStartedAt: startedAt,
		SpawnerPID:        child.PID(),
		SpawnerStartedAt:  spawnerStartedAt,
	}
	if err := service.SaveEndpoint(a.endpointPath, record); err != nil {
		a.logf("端点记录写入失败: %v", err)
	}
	a.logf("已启动自有实例 %s:%d（spawnerPid=%d listenerPid=%d）", host, port, child.PID(), listenerPID)

	target := fmt.Sprintf("http://%s:%d", host, port)
	if readyURL != "" {
		// The ready line carries the authenticated URL (the per-process launch
		// token): navigating there performs the token -> cookie exchange.
		target = readyURL
	}
	a.navigate(target)
}

// showTokenPage asks for the other instance's launch token (FR-05). The page is
// local HTML; nothing is spawned while it is up.
func (a *app) showTokenPage(host string, port int) {
	a.mu.Lock()
	a.tokenHost, a.tokenPort = host, port
	a.mu.Unlock()
	a.page(ui.Token)
}

func (a *app) tokenEndpoint() (string, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tokenHost, a.tokenPort
}

func (a *app) tokenTarget() dswebview.TokenTarget {
	host, port := a.tokenEndpoint()
	return dswebview.TokenTarget{Host: host, Port: port}
}

// onTokenSubmit validates a pasted token off the UI thread and either attaches
// to the instance or reports the reason back into the page. The token itself is
// never logged and never written to disk.
func (a *app) onTokenSubmit(pasted string) dswebview.TokenResult {
	host, port := a.tokenEndpoint()
	token := service.ParseTokenInput(pasted)
	if token == "" {
		return dswebview.TokenResult{Message: "没识别出 token：请粘贴 dsh web 打印的整条 URL（含 ?token=…），或直接粘贴 token 本身。"}
	}
	go func() {
		ok, err := service.ValidateToken(context.Background(), host, port, token)
		switch {
		case err != nil:
			a.logf("token 校验无法完成（%s:%d）: %v", host, port, err)
			a.notifyToken(false, "无法连接到该实例，请确认 dsh web 仍在运行后重试。")
		case ok:
			a.logf("token 校验通过，附着 %s:%d", host, port)
			a.notifyToken(true, "token 有效，正在进入界面…")
			a.navigate(fmt.Sprintf("http://%s:%d/?token=%s", host, port, url.QueryEscape(token)))
		default:
			a.logf("token 校验未通过（%s:%d 返回 401）", host, port)
			a.notifyToken(false, "token 无效或已过期，请重新从 dsh web 的输出复制。")
		}
	}()
	return dswebview.TokenResult{OK: true, Message: "已提交，正在校验…"}
}

// notifyToken pushes the verdict into the token page.
func (a *app) notifyToken(ok bool, message string) {
	a.eval(fmt.Sprintf("window.%s && window.%s(%t, %s)", dswebview.TokenResultFunc, dswebview.TokenResultFunc, ok, jsString(message)))
}

// onTokenSkip is the token page's "use my own instance" action: a fresh instance
// on a port >= FallbackPortMin, leaving the other instance untouched.
func (a *app) onTokenSkip() dswebview.TokenResult {
	go a.spawnOwn(0)
	return dswebview.TokenResult{OK: true, Message: "正在启动本应用自己的实例…"}
}

// onRetry re-runs the startup decision from the error page.
func (a *app) onRetry() dswebview.TokenResult {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return dswebview.TokenResult{Message: "已有启动流程在进行中…"}
	}
	a.running = true
	a.mu.Unlock()

	a.page(ui.Loading)
	go func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
		}()
		a.startup()
	}()
	return dswebview.TokenResult{OK: true, Message: "正在重试…"}
}

// onAuthFence handles dsh's 401 fence: an instance we own is restarted with a
// fresh token (its persisted cookie is gone), anybody else's gets the token page.
func (a *app) onAuthFence(pageURL string) {
	a.mu.Lock()
	if a.fenceHandled {
		a.mu.Unlock()
		return
	}
	a.fenceHandled = true
	owned, spawnerPID, spawnerStartedAt := a.owned, a.spawnerPID, a.spawnerStartedAt
	a.mu.Unlock()

	host, port := a.cfg.Host, a.cfg.Port
	if u, err := url.Parse(pageURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
		if p, err := strconv.Atoi(u.Port()); err == nil && p > 0 {
			port = p
		}
	}
	a.logf("页面命中 dsh 认证栅栏（%s:%d owned=%v）", host, port, owned)

	if !owned {
		// Somebody else's instance: never stop it, ask for its token instead.
		a.showTokenPage(host, port)
		return
	}
	// Our own instance lost its cookie: drop it (with proof, see StopOwned) and
	// start a fresh one so a new token mints a new cookie.
	record, _ := service.LoadEndpoint(a.endpointPath)
	if _, err := service.StopOwned(record, spawnerPID, spawnerStartedAt); err != nil {
		a.logf("认证栅栏自愈：未能证明可安全终止旧实例（%v），保留它并另起新实例", err)
	}
	_ = service.ClearEndpoint(a.endpointPath)
	a.setOwned(0, 0, false)
	a.spawnOwn(port)
}

// showError replaces the page with the local error page and pushes the reason
// into it.
func (a *app) showError(title, detail string) {
	a.logf("%s: %s", title, detail)
	a.page(ui.Error)
	a.eval(fmt.Sprintf("window.%s && window.%s(%s)", dswebview.ErrorFunc, dswebview.ErrorFunc, jsString(title+"\n"+detail)))
}

// shutdown honours --stop-on-exit, but only for an instance this shell started:
// a dsh the user launched themselves (or attached to with a token) is never
// stopped, and even our own instance is only killed when its identity can be
// proven (see service.StopOwned).
//
// "Ours" is decided from the in-memory spawn of this session *or* from the
// endpoint record, whose wrapper PID is the durable half: that keeps a
// re-launched shell able to stop the instance its previous run started, which is
// exactly what --stop-on-exit promises.
func (a *app) shutdown() {
	if !a.cfg.StopOnExit {
		return
	}
	a.mu.Lock()
	spawnerPID, spawnerStartedAt, owned := a.spawnerPID, a.spawnerStartedAt, a.owned
	a.mu.Unlock()

	record, hasRecord := service.LoadEndpoint(a.endpointPath)
	if !owned && !(hasRecord && record.Owned()) {
		a.logf("--stop-on-exit 已设置，但当前使用的是非本壳启动的实例：按其归属不予停止")
		return
	}

	killed, err := service.StopOwned(record, spawnerPID, spawnerStartedAt)
	switch {
	case err == nil && len(killed) > 0:
		_ = service.ClearEndpoint(a.endpointPath)
		a.logf("已按 --stop-on-exit 停止自有实例（listenerPid=%d spawnerPid=%d 终止进程=%v）",
			record.ListenerPID, spawnerPID, killed)
	case err == nil:
		a.logf("--stop-on-exit：自有实例已退出，无需停止（listenerPid=%d spawnerPid=%d）", record.ListenerPID, spawnerPID)
	case errors.Is(err, service.ErrNotOwned):
		a.logf("--stop-on-exit 未能证明可安全终止自有实例（listenerPid=%d spawnerPid=%d）；为安全计未终止任何进程",
			record.ListenerPID, spawnerPID)
	default:
		a.logf("--stop-on-exit 停止自有实例失败: %v", err)
	}
}

// identify resolves the process behind host:port and the dsh evidence found in
// its image path and command line. Ancestors are included because the npm shim's
// `cmd /c "dsh web …"` line -- the one carrying "dsh web" -- sits one level up
// from the node process that owns the socket.
func identify(host string, port int) (int, procinfo.Entry, procinfo.DshEvidence) {
	pid, err := procinfo.ListenerPID(host, port)
	if err != nil || pid <= 0 {
		return 0, procinfo.Entry{}, procinfo.DshEvidence{}
	}
	entry := procinfo.Info(pid)
	texts := []string{entry.ImagePath, entry.CommandLine}
	for _, ancestor := range procinfo.Ancestors(pid, 3) {
		if ancestor.PID == pid {
			continue
		}
		texts = append(texts, ancestor.ImagePath, ancestor.CommandLine)
	}
	return pid, entry, procinfo.LooksLikeDsh(port, texts...)
}

// jsString renders s as a JavaScript string literal.
func jsString(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(raw)
}

// reportConfigError reports a parameter/configuration error on the console (and
// in the log) and exits.
//
// A bad parameter is a command-line mistake: the invocation came from a shell,
// a shortcut, or a script, and the person who typed it is looking at a terminal.
// A native dialog would cover that terminal (and, for a console-subsystem build
// whose console was hidden, hide the message entirely), so parameter errors are
// printed to stderr and never shown in a MessageBox.
//
// The reason itself is already localized by internal/config (a mistyped flag
// even comes with a "did you mean" hint), and the flag package is silenced
// there, so the console shows one error line plus the usage -- each exactly
// once, in the system language, instead of an English problem line repeated
// under a Chinese prefix.
func reportConfigError(err error) {
	chinese := config.UseChineseUI()
	line := fmt.Sprintf("%s: %s", localize(chinese, "配置错误", "Configuration error"), err.Error())
	_ = appendLog(line)
	fmt.Fprintln(os.Stderr, line)
	fmt.Fprint(os.Stderr, config.Usage())
	os.Exit(2)
}

// hideConsole hides the console window that a console-subsystem build gets on
// double-click, so a GUI launch does not flash a black console. It only hides
// the console when it is this process's own dedicated console (launched by the
// shell/explorer with no parent console); when the process was started from an
// existing terminal and shares its console, the console is left alone so the
// parent's terminal window is not hidden.
func hideConsole() {
	procGetConsoleWindow := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	procGetConsoleProcessList := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	procShowWindow := syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")
	const swHide = 0

	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return // no console window at all
	}

	// Count processes attached to this console. >1 means the console is shared
	// with a parent terminal (started from PowerShell/cmd) so we must not hide
	// it; ==1 means it is our own dedicated console window.
	var pids [32]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n <= 1 {
		procShowWindow.Call(hwnd, swHide)
	}
}

// fatal logs to a file, shows a native error dialog, and exits. It is reserved
// for failures that happen before (or instead of) a window and are not
// command-line mistakes -- single instance, window creation. Everything a user
// can see after the window exists is carried by the embedded HTML pages, and
// parameter errors go to the console (see reportConfigError).
func fatal(code int, title, text string) {
	_ = appendLog(title + ": " + text)
	messageBox(title, text)
	os.Exit(code)
}

// appendLog writes a line to the shell's log file under the state directory.
func appendLog(line string) error {
	path := appdir.Path(appdir.LogFile)
	if path == "" {
		return errors.New("appendLog: state directory unavailable")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(time.Now().Format(time.RFC3339) + " " + line + "\n")
	return err
}

// messageBox shows a native, user-visible error dialog.
func messageBox(title, text string) {
	tt, _ := windows.UTF16PtrFromString(title)
	tx, _ := windows.UTF16PtrFromString(text)
	const mbOKIconError = 0x00000010 // MB_ICONERROR
	_, _ = windows.MessageBox(0, tx, tt, mbOKIconError)
}

// localize returns zh when the current system language is Chinese, otherwise en,
// so CLI messages can follow the system language.
func localize(chinese bool, zh, en string) string {
	if chinese {
		return zh
	}
	return en
}

// notifyUpdate reports an update result directly to the console (stdout for
// info, stderr for errors) and to the dsh-desktop log. The --update /
// --check-update commands run as plain CLI commands before any window opens, so
// results print to the console instead of a native dialog.
func notifyUpdate(title, text string, isError bool) {
	_ = appendLog(title + ": " + text)
	if isError {
		fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
		return
	}
	fmt.Printf("%s: %s\n", title, text)
}

// runUpdateCmd resolves the latest GitHub release and, for --update, downloads
// and applies it. It runs before the GUI window opens, printing results directly
// to the console (see notifyUpdate) rather than showing a native dialog.
func runUpdateCmd(cfg *config.Config) {
	chinese := config.UseChineseUI()

	// Generous ceiling for the download + failover chain; each download attempt
	// is individually bounded by the update package so a hanging host fails over
	// to the local proxy or a mirror instead of blowing the whole budget.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	// The resolve/metadata step is normally fast; give it its own short budget so
	// a hung api.github.com or SHA fetch cannot stall the whole command.
	resolveCtx, cancelResolve := context.WithTimeout(ctx, 2*time.Minute)
	exeURL, sha, latest, err := update.Resolve(resolveCtx, update.DefaultRepo, config.Version)
	cancelResolve()
	if err != nil {
		if errors.Is(err, update.ErrNoNewer) {
			notifyUpdate(localize(chinese, "更新检查", "Update check"),
				localize(chinese,
					fmt.Sprintf("dsh-desktop 已是最新版本 (%s)", config.Version),
					fmt.Sprintf("dsh-desktop is already the latest version (%s)", config.Version)),
				false)
			return
		}
		notifyUpdate(localize(chinese, "更新检查失败", "Update check failed"), err.Error(), true)
		return
	}

	notifyUpdate(localize(chinese, "发现新版本", "New version available"),
		localize(chinese,
			fmt.Sprintf("dsh-desktop %s 可更新(当前 %s)", latest, config.Version),
			fmt.Sprintf("dsh-desktop %s is available (current version: %s)", latest, config.Version)),
		false)
	if !cfg.Update {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		notifyUpdate(localize(chinese, "更新失败", "Update failed"),
			localize(chinese,
				"无法解析可执行文件路径: "+err.Error(),
				"cannot resolve the executable path: "+err.Error()),
			true)
		return
	}
	target := filepath.Join(filepath.Dir(exe), "dsh-desktop.exe")
	staged := target + ".new"

	if err := update.Download(ctx, exeURL, sha, staged); err != nil {
		notifyUpdate(localize(chinese, "更新下载失败", "Update download failed"), err.Error(), true)
		return
	}
	notifyUpdate(localize(chinese, "更新完成", "Update complete"),
		localize(chinese,
			fmt.Sprintf("已下载并校验 %s。更新将在本进程退出后应用,请重新运行 dsh-desktop 以使用新版本。", latest),
			fmt.Sprintf("Downloaded and verified %s. The update will be applied after this process exits; re-run dsh-desktop to use the new version.", latest)),
		false)

	if err := applyUpdate(staged, target); err != nil {
		notifyUpdate(localize(chinese, "更新未完成", "Update not complete"),
			localize(chinese,
				fmt.Sprintf("已暂存到 \"%s\",请手动替换 \"%s\" 后重新运行。\n错误: %v", staged, target, err),
				fmt.Sprintf("Staged to \"%s\". Manually replace \"%s\" and re-run.\nError: %v", staged, target, err)),
			true)
		return
	}
}

// applyUpdate swaps staged (the newly downloaded exe) over target using a
// detached helper that waits for this process to exit before copying. It does
// NOT relaunch the app: --update is a plain CLI command, so it applies the new
// binary and exits, leaving the user to start dsh-desktop again. This matches
// the flag help ("download and apply the latest release, then exit").
func applyUpdate(staged, target string) error {
	helper := target + ".update.cmd"
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW

	script := "@echo off\r\n" +
		"timeout /t 2 /nobreak >nul\r\n" +
		"copy /y \"" + staged + "\" \"" + target + "\" >nul\r\n" +
		"del \"" + staged + "\" >nul 2>nul\r\n" +
		"del \"%~f0\" >nul 2>nul\r\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/c", helper)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(helper)
		return err
	}
	return nil
}
