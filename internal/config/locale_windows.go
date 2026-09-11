//go:build windows

package config

import "syscall"

// systemUILanguage returns the user-interface language identifier reported by
// the OS (GetUserDefaultUILanguage) and true, or (0, false) when the API is
// unavailable.
func systemUILanguage() (uint16, bool) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")
	if err := proc.Find(); err != nil {
		return 0, false
	}
	r, _, _ := proc.Call()
	return uint16(r), true
}
