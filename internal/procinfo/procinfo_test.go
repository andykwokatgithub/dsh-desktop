package procinfo

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestListenerPIDForOwnListener proves the PID lookup end to end against a
// socket this test process really owns.
func TestListenerPIDForOwnListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected address type %T", ln.Addr())
	}
	got, err := ListenerPID("127.0.0.1", addr.Port)
	if err != nil {
		t.Fatalf("ListenerPID(%d): %v", addr.Port, err)
	}
	if want := os.Getpid(); got != want {
		t.Fatalf("ListenerPID(%d) = %d, want %d (this test process)", addr.Port, got, want)
	}
}

// TestListenerPIDWildcardBind covers the fallback: a listener bound to every
// interface still resolves when the probe names 127.0.0.1.
func TestListenerPIDWildcardBind(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot bind a wildcard port: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected address type %T", ln.Addr())
	}
	got, err := ListenerPID("127.0.0.1", addr.Port)
	if err != nil {
		t.Fatalf("ListenerPID(%d) on a wildcard listener: %v", addr.Port, err)
	}
	if want := os.Getpid(); got != want {
		t.Fatalf("ListenerPID(%d) = %d, want %d", addr.Port, got, want)
	}
}

func TestListenerPIDUnknownPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected address type %T", ln.Addr())
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if pid, err := ListenerPID("127.0.0.1", addr.Port); err == nil {
		t.Fatalf("ListenerPID(%d) = %d, want an error after the listener closed", addr.Port, pid)
	}
	if _, err := ListenerPID("127.0.0.1", 0); err == nil {
		t.Fatal("ListenerPID with port 0 should be rejected")
	}
}

// TestInfoOwnProcess exercises the image path, command line, start time and
// parent lookup against this process, which always exists.
func TestInfoOwnProcess(t *testing.T) {
	pid := os.Getpid()
	e := Info(pid)

	if e.PID != pid {
		t.Fatalf("PID = %d, want %d", e.PID, pid)
	}
	base := strings.ToLower(filepath.Base(e.ImagePath))
	if !strings.Contains(base, "procinfo") || !strings.HasSuffix(base, ".exe") {
		t.Fatalf("ImagePath = %q, want this test binary", e.ImagePath)
	}
	if !strings.Contains(strings.ToLower(e.CommandLine), "procinfo") {
		t.Fatalf("CommandLine = %q, want this test binary's command line", e.CommandLine)
	}
	if e.StartedAt.IsZero() || e.StartedAt.After(time.Now()) {
		t.Fatalf("StartedAt = %v, want a past timestamp", e.StartedAt)
	}
	if e.ParentPID <= 0 {
		t.Fatalf("ParentPID = %d, want the test runner", e.ParentPID)
	}
}

func TestAncestorsOwnProcess(t *testing.T) {
	got := Ancestors(os.Getpid(), 2)
	if len(got) == 0 || got[0].PID != os.Getpid() {
		t.Fatalf("Ancestors() = %+v, want this process first", got)
	}
	for i := 1; i < len(got); i++ {
		// A real ancestor is created no later than its descendant.
		if !got[i].StartedAt.IsZero() && !got[0].StartedAt.IsZero() && got[i].StartedAt.After(got[0].StartedAt) {
			t.Fatalf("ancestor %d started after its descendant: %+v", got[i].PID, got)
		}
	}
}

// TestIsChildOf pins the parent proof used before any kill (see
// service.StopOwned): only a process we actually started has us as its parent.
func TestIsChildOf(t *testing.T) {
	self := os.Getpid()
	parent, err := ParentPID(self)
	if err != nil {
		t.Fatalf("ParentPID: %v", err)
	}
	if !IsChildOf(self, parent) {
		t.Fatalf("IsChildOf(%d, %d) = false, want true", self, parent)
	}
	if IsChildOf(self, self) {
		t.Fatal("a process must not count as its own child")
	}
	if IsChildOf(0, parent) || IsChildOf(self, 0) {
		t.Fatal("invalid PIDs must never be accepted")
	}
	if IsChildOf(self, self+1_000_000) {
		t.Fatal("an unrelated parent must not be accepted")
	}
}

// TestNetworkPort pins the byte-order decoding of the MIB tables.
func TestNetworkPort(t *testing.T) {
	cases := []struct {
		raw  uint32
		want int
	}{
		{0x080C, 3080}, // 3080 = 0x0C08 in network order, read little-endian
		{0x1400, 20},
		{0x5000, 80},
		{0xFFFF, 65535},
	}
	for _, tc := range cases {
		if got := networkPort(tc.raw); got != tc.want {
			t.Fatalf("networkPort(0x%04X) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}
