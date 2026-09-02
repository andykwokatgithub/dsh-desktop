// Package singleinstance enforces a single running instance using a mutex and
// a uniquely-classed native window, and activates the existing instance via a
// registered window message.
//
// Design notes (PRD V1.1):
//   - D2: use `Local\` so multi-user/RDP sessions can each run an instance.
//   - D3: locate the existing window by a unique window class (not by title)
//     and activate it with a RegisterWindowMessage.
package singleinstance

import (
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	// ClassName is the unique window class registered for the shell window.
	// It is used (not the title) to find and activate the existing instance.
	ClassName = "DSHDesktopAppMainWindow"
	// MutexName is the per-session mutex guarding a single instance.
	MutexName = `Local\DSHDesktopApp`
	// ActivateMessage is the registered message used to bring the existing
	// window forward.
	ActivateMessage = "DSHDesktopApp.Activate"
)

// Acquire tries to create the single-instance mutex. If it already exists it
// returns ok=false, meaning another instance is running.
func Acquire() (winHandle windows.Handle, ok bool, err error) {
	name, err := windows.UTF16PtrFromString(MutexName)
	if err != nil {
		return 0, false, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if err == syscall.ERROR_ALREADY_EXISTS {
			return h, false, nil
		}
		return 0, false, err
	}
	return h, true, nil
}

// ActivateExisting finds the anchor window of a running instance and asks it to
// restore/foreground itself. It returns false when no such window is found.
func ActivateExisting() (bool, error) {
	cls, err := windows.UTF16PtrFromString(ClassName)
	if err != nil {
		return false, err
	}
	hwnd, err := findWindow(cls, nil)
	if err != nil || hwnd == 0 {
		return false, nil
	}
	msg := registerWindowMessage(mustUTF16(ActivateMessage))
	sendMessageTimeout(hwnd, msg, 0, 0, uintptr(smtoAbortIfHung), 3000)
	return true, nil
}

func mustUTF16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}
