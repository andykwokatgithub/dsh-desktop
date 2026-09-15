// Package dpi declares the process DPI-awareness level and provides helpers
// to convert between logical (CSS) and physical pixel sizes.
//
// The process DPI awareness must be set before ANY window is created.
// webview_go only calls enable_dpi_awareness() when it owns the window
// (m_owns_window == true); since dsh-desktop passes its own HWND to
// webview.NewWindow, the library skips that call entirely. Without an
// early SetProcessDpiAwarenessContext call, Windows bitmap-scales the
// whole window and WebView2 text renders blurry.
package dpi

import "golang.org/x/sys/windows"

// DPI awareness context handle values (Windows 10 1703+).
// These are negative integers reinterpreted as HANDLE (DPI_AWARENESS_CONTEXT).
const (
	dpiAwarenessContextPerMonitorV2 = ^uintptr(3) // -4: DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	dpiAwarenessContextPerMonitor   = ^uintptr(2) // -3: DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE
)

// shcore PROCESS_DPI_AWARENESS (Windows 10 1607+).
const processPerMonitorDpiAware = 2

// E_ACCESSDENIED (0x80070005): SetProcessDpiAwareness returns this when the
// process is already DPI-aware. Treat it as success.
const eAccessDenied = 0x80070005

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDpiAwareness        = shcore.NewProc("SetProcessDpiAwareness")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")

	// GetDpiForSystem is exported by user32.dll (Windows 10 1607+), NOT by
	// shcore.dll (shcore exports GetDpiForMonitor). Looking it up in the wrong
	// module makes LazyProc.Addr() panic with "Failed to find GetDpiForSystem
	// procedure in shcore.dll".
	procGetDpiForSystem = user32.NewProc("GetDpiForSystem")
)

// available reports whether a lazily-resolved export actually exists on this
// system. LazyProc.Addr()/Call() panic (via mustFind) when the export is
// missing, so probe with Find(), which returns an error instead, and let the
// caller fall back gracefully on older Windows builds.
func available(p *windows.LazyProc) bool {
	return p.Find() == nil
}

// SetAwareness declares the process DPI-awareness level. It must be called
// before any window is created, otherwise Windows bitmap-scales the entire
// window, making WebView2 text blurry on high-DPI displays.
//
// Preference order (mirrors webview.h's enable_dpi_awareness):
//  1. Per-monitor V2 (Win10 1703+): highest fidelity, handles mixed-DPI.
//  2. Per-monitor V1 (Win10 1607+): good fidelity.
//  3. System-level (Vista+): basic awareness, avoids bitmap scaling.
func SetAwareness() {
	// 1) SetProcessDpiAwarenessContext — per-monitor V2, then V1.
	if available(procSetProcessDpiAwarenessContext) {
		if r, _, _ := procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorV2); r != 0 {
			return
		}
		if r, _, _ := procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitor); r != 0 {
			return
		}
	}

	// 2) SetProcessDpiAwareness — shcore, per-monitor V1.
	if available(procSetProcessDpiAwareness) {
		if r, _, _ := procSetProcessDpiAwareness.Call(processPerMonitorDpiAware); r == 0 || r == eAccessDenied {
			return
		}
	}

	// 3) SetProcessDPIAware — system-level (Vista+).
	if available(procSetProcessDPIAware) {
		procSetProcessDPIAware.Call()
	}
}

// SystemDpi returns the primary monitor's DPI, or 96 (USER_DEFAULT_SCREEN_DPI)
// if the API is unavailable. Call SetAwareness() first so the returned value
// reflects the actual DPI rather than the fake 96 Windows reports to
// DPI-unaware processes.
func SystemDpi() int {
	if available(procGetDpiForSystem) {
		if r, _, _ := procGetDpiForSystem.Call(); r > 0 {
			return int(r)
		}
	}
	return 96
}

// ScaleToDpi converts a value from one DPI to another using integer
// arithmetic with rounding. Either DPI <= 0 defaults to 96.
func ScaleToDpi(value, fromDpi, toDpi int) int {
	if fromDpi <= 0 {
		fromDpi = 96
	}
	if toDpi <= 0 {
		toDpi = 96
	}
	return (value*toDpi + fromDpi/2) / fromDpi
}
