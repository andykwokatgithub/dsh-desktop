// dsh-desktop is a Go + WebView2 desktop shell for the DeepSeek Harness Web UI.
//
// It manages the dsh web backend (probe/spawn/lifecycle), enforces a single
// running instance, and embeds the UI in a native window.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/deepseek-ai/dsh-desktop/internal/config"
	"github.com/deepseek-ai/dsh-desktop/internal/service"
	"github.com/deepseek-ai/dsh-desktop/internal/singleinstance"
	"github.com/deepseek-ai/dsh-desktop/internal/ui"
	"github.com/deepseek-ai/dsh-desktop/internal/update"
	dswebview "github.com/deepseek-ai/dsh-desktop/internal/webview"
	"golang.org/x/sys/windows"
)

func main() {
	// webview_go requires the window and message loop on a locked OS thread.
	runtime.LockOSThread()

	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fatal(2, "配置错误", err.Error())
	}
	if cfg.ShowVersion {
		fmt.Printf("dsh-desktop %s\n", config.Version)
		os.Exit(0)
	}

	// Update path: handled as a plain CLI command before the GUI/single-instance
	// path, so -check-update / -update never open a window.
	if cfg.CheckUpdate || cfg.Update {
		runUpdateCmd(cfg)
		os.Exit(0)
	}

	// FR-03: single instance. If another is running, activate it and exit.
	_, ok, err := singleinstance.Acquire()
	if err != nil {
		fatal(1, "单实例错误", err.Error())
	}
	if !ok {
		_, _ = singleinstance.ActivateExisting()
		os.Exit(0)
	}

	// FR-01/FR-04: ensure the dsh web service is (or becomes) healthy.
	beh := service.Behavior{
		URL:            cfg.URL,
		Host:           cfg.Host,
		Port:           cfg.Port,
		Command:        cfg.Command,
		StartupTimeout: time.Duration(cfg.StartupTimeout) * time.Second,
		PollInterval:   time.Duration(cfg.PollIntervalMS) * time.Millisecond,
	}

	var svc *service.Cmd
	if !beh.Healthy() {
		if beh.PortOpen() {
			// Port is open but not a healthy dsh service -> do NOT restart (AC-06).
			fatal(1, "端口冲突", fmt.Sprintf("%s:%d 已被其它进程占用，无法启动 dsh（不会自动重启）。", cfg.Host, cfg.Port))
		}
		ctx := context.Background()
		svc, err = service.Spawn(ctx, beh)
		if err != nil {
			fatal(1, "无法启动 DeepSeek Harness", fmt.Sprintf("启动失败: %v\n请确认已全局安装 dsh 命令（npm i -g @deepseek-ai/dsh）且 node、dsh 均位于 PATH。", err))
		}
	}

	// FR-02: create the native window and embed WebView2, starting on the
	// loading page so the window appears within the cold-start budget.
	view, _, err := singleinstance.NewWindow(singleinstance.WindowOptions{
		Title:  cfg.WindowTitle,
		Width:  cfg.WindowWidth,
		Height: cfg.WindowHeight,
		Debug:  cfg.DevTools,
	})
	if err != nil {
		fatal(1, "窗口创建失败", err.Error())
	}
	view.Init(dswebview.SecurityInit(cfg.ContextMenu))
	_ = dswebview.BindExternal(view)
	view.SetHtml(ui.Loading)

	if svc == nil {
		// Service was already running and healthy; go straight to the UI.
		view.Navigate(cfg.URL)
	} else {
		// Wait on a background goroutine and hop back to the UI thread.
		ctx := context.Background()
		go func() {
			if err := service.WaitHealthy(ctx, beh, svc); err != nil {
				_ = service.Stop(svc.PID())
				view.Dispatch(func() { view.SetHtml(ui.Error) })
				return
			}
			view.Dispatch(func() { view.Navigate(cfg.URL) })
		}()
	}

	// Block until WM_CLOSE triggers Terminate (see singleinstance.WndProc).
	view.Run()

	// Cleanup (FR-04): stop the service we started only when asked to.
	view.Destroy()
	if cfg.StopOnExit && svc != nil {
		_ = service.Stop(svc.PID())
	}
	os.Exit(0)
}

// fatal logs to a file, shows a native error dialog, and exits. The process is
// GUI-subsystem (no console), so stderr is not user-visible.
func fatal(code int, title, text string) {
	_ = appendLog(title + ": " + text)
	messageBox(title, text)
	os.Exit(code)
}

// appendLog writes a line to the log file under %LOCALAPPDATA%\dsh-desktop.
func appendLog(line string) error {
	dir := os.Getenv("LOCALAPPDATA")
	logDir := filepath.Join(dir, "dsh-desktop")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, "dsh-desktop.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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

// notifyUpdate reports an update result to the user. It always writes to the
// dsh-desktop log, prints to stdout (visible under `go run`), and shows a native
// dialog (visible even though the GUI binary has no console).
func notifyUpdate(title, text string, isError bool) {
	_ = appendLog(title + ": " + text)
	fmt.Printf("%s: %s\n", title, text)
	flags := uint32(0x00000040) // MB_ICONINFORMATION
	if isError {
		flags = 0x00000010 // MB_ICONERROR
	}
	tt, _ := windows.UTF16PtrFromString(title)
	tx, _ := windows.UTF16PtrFromString(text)
	_, _ = windows.MessageBox(0, tx, tt, flags)
}

// runUpdateCmd resolves the latest GitHub release and, for -update, downloads
// and applies it. It runs before the GUI window opens; results are shown via a
// native dialog (and the log) so the user sees feedback even though the GUI
// subsystem has no console.
func runUpdateCmd(cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	exeURL, sha, latest, err := update.Resolve(ctx, update.DefaultRepo, config.Version)
	if err != nil {
		if errors.Is(err, update.ErrNoNewer) {
			notifyUpdate("更新检查", fmt.Sprintf("dsh-desktop 已是最新版本 (%s)", config.Version), false)
			return
		}
		notifyUpdate("更新检查失败", err.Error(), true)
		return
	}

	notifyUpdate("发现新版本", fmt.Sprintf("dsh-desktop %s 可更新(当前 %s)", latest, config.Version), false)
	if !cfg.Update {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		notifyUpdate("更新失败", "无法解析可执行文件路径: "+err.Error(), true)
		return
	}
	target := filepath.Join(filepath.Dir(exe), "dsh-desktop.exe")
	staged := target + ".new"

	if err := update.Download(ctx, exeURL, sha, staged); err != nil {
		notifyUpdate("更新下载失败", err.Error(), true)
		return
	}
	notifyUpdate("更新完成", fmt.Sprintf("已下载并校验 %s,正在应用后重启...", latest), false)

	if err := applyUpdate(staged, target); err != nil {
		notifyUpdate("更新未完成", fmt.Sprintf("已暂存到 \"%s\",请手动替换 \"%s\" 后重启。\n错误: %v", staged, target, err), true)
		return
	}
}

// applyUpdate swaps staged (the newly downloaded exe) over target using a
// detached helper that waits for this process to exit before copying, then
// relaunches the app. Runs only on the opt-in -update CLI path.
func applyUpdate(staged, target string) error {
	helper := target + ".update.cmd"
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW

	script := "@echo off\r\n" +
		"timeout /t 2 /nobreak >nul\r\n" +
		"copy /y \"" + staged + "\" \"" + target + "\" >nul\r\n" +
		"del \"" + staged + "\" >nul 2>nul\r\n" +
		"start \"\" \"" + target + "\"\r\n" +
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
