package singleinstance

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// x/sys/windows does not export most user32 window functions, so they are
// loaded lazily here. Constants the package also omits are defined locally.

const (
	cwUseDefault    int32  = -2147483648 // CW_USEDEFAULT
	gwlUserData     int32  = -21         // GWLP_USERDATA
	wsOverlappedWnd uint32 = 0x00CF0000  // WS_OVERLAPPEDWINDOW
	wmSize          uint32 = 0x0005
	wmClose         uint32 = 0x0010
	wmSetIcon       uint32 = 0x0080
	swShow          int32  = 5   // SW_SHOW
	swRestore       int32  = 9   // SW_RESTORE
	smtoAbortIfHung uint32 = 0x0002
	imageIcon       uint32 = 1   // IMAGE_ICON
	iconBig         uintptr = 1   // ICON_BIG
	iconSmall       uintptr = 0   // ICON_SMALL
)

// wndClassEx mirrors WNDCLASSEX (not exported by x/sys/windows).
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procGetModuleHandle       = kernel32.NewProc("GetModuleHandleW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procFindWindowW           = user32.NewProc("FindWindowW")
	procRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	procSendMessageTimeoutW   = user32.NewProc("SendMessageTimeoutW")
	procFindWindowExW         = user32.NewProc("FindWindowExW")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procMoveWindow            = user32.NewProc("MoveWindow")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procIsIconic              = user32.NewProc("IsIconic")
	procAttachThreadInput     = user32.NewProc("AttachThreadInput")
	procShowWindow            = user32.NewProc("ShowWindow")
	procLoadImageW            = user32.NewProc("LoadImageW")
	procSendMessageW          = user32.NewProc("SendMessageW")
)

func getModuleHandle() (uintptr, error) {
	r, _, err := procGetModuleHandle.Call(0)
	if r == 0 {
		return 0, err
	}
	return r, nil
}

func registerClassEx(wc *wndClassEx) bool {
	r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wc)))
	return r != 0
}

func createWindowEx(exStyle uint32, className, windowName *uint16, style uint32,
	x, y, width, height int32, parent, menu, instance uintptr, param unsafe.Pointer) (uintptr, error) {
	r, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		uintptr(style),
		uintptr(uint32(x)), uintptr(uint32(y)), uintptr(uint32(width)), uintptr(uint32(height)),
		parent, menu, instance, uintptr(param))
	if r == 0 {
		return 0, err
	}
	return r, nil
}

func defWindowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func findWindow(className, windowName *uint16) (uintptr, error) {
	r, _, err := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)))
	if r == 0 {
		return 0, err
	}
	return r, nil
}

func registerWindowMessage(name *uint16) uint32 {
	r, _, _ := procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(name)))
	return uint32(r)
}

func sendMessageTimeout(hwnd uintptr, msg uint32, wparam, lparam, flags, timeout uintptr) uintptr {
	var result uintptr
	_, _, _ = procSendMessageTimeoutW.Call(hwnd, uintptr(msg), wparam, lparam, flags, timeout,
		uintptr(unsafe.Pointer(&result)))
	return result
}

func findWindowEx(parent, childAfter uintptr, className, windowName *uint16) (uintptr, error) {
	r, _, err := procFindWindowExW.Call(parent, childAfter,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)))
	if r == 0 {
		return 0, err
	}
	return r, nil
}

func getClientRect(hwnd uintptr, rect *windows.Rect) bool {
	r, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(rect)))
	return r != 0
}

func moveWindow(hwnd uintptr, x, y, width, height int32, repaint bool) bool {
	repaintv := uintptr(0)
	if repaint {
		repaintv = 1
	}
	r, _, _ := procMoveWindow.Call(hwnd, uintptr(uint32(x)), uintptr(uint32(y)),
		uintptr(uint32(width)), uintptr(uint32(height)), repaintv)
	return r != 0
}

func setWindowLongPtr(hwnd uintptr, index int32, value uintptr) uintptr {
	r, _, _ := procSetWindowLongPtrW.Call(hwnd, uintptr(uint32(index)), value)
	return r
}

func getWindowLongPtr(hwnd uintptr, index int32) uintptr {
	r, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(uint32(index)))
	return r
}

func setForegroundWindow(hwnd uintptr) bool {
	r, _, _ := procSetForegroundWindow.Call(hwnd)
	return r != 0
}

func isIconic(hwnd uintptr) bool {
	r, _, _ := procIsIconic.Call(hwnd)
	return r != 0
}

func attachThreadInput(idAttach, idAttachTo uint32, attach bool) bool {
	av := uintptr(0)
	if attach {
		av = 1
	}
	r, _, _ := procAttachThreadInput.Call(uintptr(idAttach), uintptr(idAttachTo), av)
	return r != 0
}

func showWindow(hwnd uintptr, cmd int32) bool {
	r, _, _ := procShowWindow.Call(hwnd, uintptr(uint32(cmd)))
	return r != 0
}

// loadImageIcon loads an icon from the module instance resource (e.g. the
// embedded icon group ID 1) at a specific size.
func loadImageIcon(instance, resID uintptr, cx, cy int32) uintptr {
	r, _, _ := procLoadImageW.Call(instance, resID, uintptr(imageIcon),
		uintptr(uint32(cx)), uintptr(uint32(cy)), 0)
	return r
}

// sendMessage posts a window message synchronously (used for WM_SETICON).
func sendMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
