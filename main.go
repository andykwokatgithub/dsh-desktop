// dsh-desktop is a Go + WebView2 desktop shell for the DeepSeek Harness Web UI.
//
// It manages the dsh web backend (probe/spawn/lifecycle), enforces a single
// running instance, and embeds the UI in a native window.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/deepseek-ai/dsh-desktop/internal/config"
	"github.com/deepseek-ai/dsh-desktop/internal/service"
	"github.com/deepseek-ai/dsh-desktop/internal/singleinstance"
	"github.com/deepseek-ai/dsh-desktop/internal/ui"
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
