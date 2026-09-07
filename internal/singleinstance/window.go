package singleinstance

import (
	"sync"
	"syscall"
	"unsafe"

	webview "github.com/webview/webview_go"
	"golang.org/x/sys/windows"
)

// WindowOptions configures the shell window and its embedded WebView2 view.
type WindowOptions struct {
	Title    string
	Width    int
	Height   int
	DevTools bool // expose WebView2 devtools (default off per D7)
	OnClose  func()
}

// Window is the native shell window that hosts the WebView2 content.
type Window struct {
	hwnd uintptr
	opts WindowOptions
	view webview.WebView
}

var activateMsg uint32

// GWLP_USERDATA holds an integer handle, not a Go pointer, so the GC can track
// the actual *Window via this registry (avoiding a raw Go pointer in native
// memory, which GC cannot scan).
var (
	winMu   sync.Mutex
	winNext uintptr
	winReg  = map[uintptr]*Window{}
)

// NewWindow creates the shell window and embeds a WebView2 view into it. It is
// the exported entry point used by main.
func NewWindow(opts WindowOptions) (webview.WebView, *Window, error) {
	return newWindow(opts)
}

func newWindow(opts WindowOptions) (webview.WebView, *Window, error) {
	instance, err := getModuleHandle()
	if err != nil {
		return nil, nil, err
	}

	clsName := mustUTF16(ClassName)
	activateMsg = registerWindowMessage(mustUTF16(ActivateMessage))

	// Load the icon embedded in this executable (rsrc places it as ID 1) and
	// assign it to the window class so the title bar and taskbar show it.
	const iconResourceID = 1 // IDI_ICON1
	bigIcon := loadImageIcon(instance, iconResourceID, 32, 32)
	smallIcon := loadImageIcon(instance, iconResourceID, 16, 16)

	wc := &wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  instance,
		Icon:      bigIcon,
		IconSm:    smallIcon,
		ClassName: clsName,
	}
	_ = registerClassEx(wc)

	win := &Window{opts: opts}
	hwnd, err := createWindowEx(0, clsName, mustUTF16(opts.Title), wsOverlappedWnd,
		cwUseDefault, cwUseDefault, int32(opts.Width), int32(opts.Height),
		0, 0, instance, nil)
	if err != nil {
		return nil, nil, err
	}
	win.hwnd = hwnd

	// Belt-and-suspenders: set the window icon explicitly for the title bar
	// (ICON_SMALL) and the taskbar (ICON_BIG).
	sendMessage(hwnd, wmSetIcon, iconSmall, smallIcon)
	sendMessage(hwnd, wmSetIcon, iconBig, bigIcon)

	winMu.Lock()
	winNext++
	id := winNext
	winReg[id] = win
	winMu.Unlock()
	setWindowLongPtr(hwnd, gwlUserData, id)

	showWindow(hwnd, swShow)

	// Embed a WebView2 view into our own window (D3: we own the window class).
	view := webview.NewWindow(opts.DevTools, unsafe.Pointer(hwnd))
	win.view = view
	view.SetTitle(opts.Title)
	view.SetSize(opts.Width, opts.Height, webview.HintNone)
	return view, win, nil
}

// HWND returns the native window handle.
func (w *Window) HWND() windows.HWND { return windows.HWND(w.hwnd) }

// View returns the embedded WebView2 view.
func (w *Window) View() webview.WebView { return w.view }

// BringToFront restores and foregrounds the window. Called by the WndProc when
// the activation message arrives from a second instance.
func (w *Window) BringToFront() {
	if isIconic(w.hwnd) {
		showWindow(w.hwnd, swRestore)
	}
	activateWindow(w.hwnd)
}

func lookupWindow(hwnd uintptr) *Window {
	id := getWindowLongPtr(hwnd, gwlUserData)
	winMu.Lock()
	defer winMu.Unlock()
	return winReg[id]
}

func wndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	win := lookupWindow(hwnd)

	switch msg {
	case uintptr(activateMsg):
		if win != nil {
			win.BringToFront()
		}
		return 0

	case uintptr(wmSize):
		resizeChild(windows.HWND(hwnd))
		return 0

	case uintptr(wmClose):
		if win != nil && win.opts.OnClose != nil {
			win.opts.OnClose()
		}
		if win != nil && win.view != nil {
			// Terminate() is safe from any thread and breaks the Run loop.
			win.view.Terminate()
		}
		return 0
	}
	return defWindowProc(hwnd, uint32(msg), wparam, lparam)
}

// resizeChild keeps the embedded WebView2 child filling the client area.
func resizeChild(parent windows.HWND) {
	child, err := findWindowEx(uintptr(parent), 0, mustUTF16("webview_widget"), nil)
	if err != nil || child == 0 {
		return
	}
	var rc windows.Rect
	if !getClientRect(uintptr(parent), &rc) {
		return
	}
	moveWindow(child, 0, 0, rc.Right-rc.Left, rc.Bottom-rc.Top, true)
}

// activateWindow brings hwnd to the foreground WITHOUT changing its size or
// position. It never calls ShowWindow(SW_RESTORE): restoring a minimized window
// is the responsibility of BringToFront, and doing it here unconditionally was
// what could reset the window's geometry on a second-instance activation.
func activateWindow(hwnd uintptr) {
	if setForegroundWindow(hwnd) {
		return
	}
	fg := windows.GetForegroundWindow()
	curThread := windows.GetCurrentThreadId()
	fgThread, _ := windows.GetWindowThreadProcessId(fg, nil)
	if fgThread != 0 && fgThread != curThread {
		attached := attachThreadInput(curThread, fgThread, true)
		if attached {
			defer attachThreadInput(curThread, fgThread, false)
		}
	}
	setForegroundWindow(hwnd)
}
