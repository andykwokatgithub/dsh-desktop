package service

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepseek-ai/dsh-desktop/internal/procinfo"
)

// TestEndpointRecordAndLiveness uses this test process as the stand-in for the
// node.exe that owns a real listening socket: the record's authority comes from
// the PID plus its creation time, which is exactly what survives PID reuse.
func TestEndpointRecordAndLiveness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-endpoint.json")

	if _, ok := LoadEndpoint(path); ok {
		t.Fatal("a missing record should not load")
	}
	if _, ok := LoadEndpoint(""); ok {
		t.Fatal("an empty path should not load")
	}

	started, err := procinfo.StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("StartTime: %v", err)
	}
	record := Endpoint{
		Host:              "127.0.0.1",
		Port:              3080,
		ListenerPID:       os.Getpid(),
		ListenerStartedAt: started.UnixNano(),
		SpawnerPID:        4321,
		SpawnerStartedAt:  started.UnixNano(),
	}
	if err := SaveEndpoint(path, record); err != nil {
		t.Fatalf("SaveEndpoint: %v", err)
	}
	got, ok := LoadEndpoint(path)
	if !ok {
		t.Fatal("the record did not load back")
	}
	if got.Port != 3080 || got.ListenerPID != os.Getpid() || got.SpawnerPID != 4321 || got.Host != "127.0.0.1" {
		t.Fatalf("LoadEndpoint() = %+v, want the saved record", got)
	}
	if got.SpawnerStartedAt != record.SpawnerStartedAt {
		t.Fatalf("SpawnerStartedAt = %d, want %d", got.SpawnerStartedAt, record.SpawnerStartedAt)
	}

	if !record.Live("127.0.0.1", 3080) {
		t.Fatal("a record for this live process should be Live")
	}
	if record.Live("127.0.0.1", 3081) {
		t.Fatal("a different port must not match")
	}
	if record.Live("127.0.0.2", 3080) {
		t.Fatal("a different host must not match")
	}
	stale := record
	stale.ListenerStartedAt += int64(2 * time.Hour)
	if stale.Live("127.0.0.1", 3080) {
		t.Fatal("a mismatched start time must not match (PID reuse guard)")
	}
	dead := record
	dead.ListenerPID = 0
	if dead.Live("127.0.0.1", 3080) {
		t.Fatal("a record without a listener PID must not match")
	}

	if err := ClearEndpoint(path); err != nil {
		t.Fatalf("ClearEndpoint: %v", err)
	}
	if _, ok := LoadEndpoint(path); ok {
		t.Fatal("a cleared record still loads")
	}
	if err := ClearEndpoint(path); err != nil {
		t.Fatalf("ClearEndpoint on a missing file: %v", err)
	}
}

func TestLoadEndpointRejectsMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadEndpoint(path); ok {
		t.Fatal("malformed JSON should not load")
	}
	if err := os.WriteFile(path, []byte(`{"host":"127.0.0.1","port":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadEndpoint(path); ok {
		t.Fatal("a record without a port should not load")
	}
}

func TestListenerMemoryWithoutListener(t *testing.T) {
	// A port nothing listens on must degrade to zeroes rather than to a wrong
	// identity.
	pid, startedAt := ListenerMemory("127.0.0.1", 1)
	if pid != 0 || startedAt != 0 {
		t.Fatalf("ListenerMemory on an unused port = (%d, %d), want zeroes", pid, startedAt)
	}
}

// freeListeningPort returns a port this test process really listens on, so
// procinfo.ListenerPID resolves it as the endpoint's owner.
func freeListeningPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected address type %T", ln.Addr())
	}
	return addr.Port
}

// silentPort returns a port that was just released, so nothing listens on it.
func silentPort(t *testing.T) int {
	t.Helper()
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
	return addr.Port
}

// startThrowawayChild starts a child that lives long enough to be killed by the
// tests below.
func startThrowawayChild(t *testing.T) *exec.Cmd {
	t.Helper()
	child := exec.Command("cmd", "/c", "ping -n 60 127.0.0.1 >nul")
	if err := child.Start(); err != nil {
		t.Skipf("cannot start a throwaway child: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
	})
	return child
}

// waitChildGone blocks until the child has really exited (its handle stays open
// here, so PID liveness is not a reliable exit signal).
func waitChildGone(t *testing.T, child *exec.Cmd) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_, _ = child.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the child is still alive")
	}
}

// TestStopOwnedRefusesUnprovenEndpoint is the safety net behind --stop-on-exit: a
// live endpoint whose process is neither this shell's child nor matched by a
// recorded creation time is never killed, so a recycled PID cannot take down an
// unrelated process.
//
// (If this wrongly killed by PID, this test process would die and the test would
// fail loudly.)
func TestStopOwnedRefusesUnprovenEndpoint(t *testing.T) {
	stale := time.Now().Add(-time.Hour).UnixNano()
	port := freeListeningPort(t)

	// The endpoint is served by this very process, but the record's creation time
	// for it is wrong -- a recycled PID. Nothing may be killed.
	killed, err := StopOwned(Endpoint{Host: "127.0.0.1", Port: port, ListenerPID: os.Getpid(), ListenerStartedAt: stale}, 0, 0)
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("StopOwned(stale listener on a live endpoint) = (%v, %v), want ErrNotOwned", killed, err)
	}
	if len(killed) != 0 {
		t.Fatalf("StopOwned killed %v, want nothing", killed)
	}

	// Same for a session spawner that cannot be proven: refused, nothing killed.
	if _, err := StopOwned(Endpoint{Host: "127.0.0.1", Port: port}, os.Getpid(), stale); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("StopOwned with a stale session spawner = %v, want ErrNotOwned", err)
	}
	if _, err := StopOwned(Endpoint{Host: "127.0.0.1", Port: port, SpawnerPID: os.Getpid(), SpawnerStartedAt: stale}, 0, 0); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("StopOwned with an unproven wrapper = %v, want ErrNotOwned", err)
	}
}

// TestStopOwnedNothingToStop covers the honest "already gone" answer: no proof is
// needed when nothing answers on the recorded endpoint any more, and no error is
// raised either.
func TestStopOwnedNothingToStop(t *testing.T) {
	stale := time.Now().Add(-time.Hour).UnixNano()
	silent := silentPort(t)

	killed, err := StopOwned(Endpoint{Host: "127.0.0.1", Port: silent, ListenerPID: 999_999, ListenerStartedAt: stale}, 0, 0)
	if err != nil || len(killed) != 0 {
		t.Fatalf("StopOwned on a silent endpoint = (%v, %v), want (nothing, nil)", killed, err)
	}
	if killed, err := StopOwned(Endpoint{}, 0, 0); err != nil || len(killed) != 0 {
		t.Fatalf("StopOwned with no PID = (%v, %v), want (nothing, nil)", killed, err)
	}
}

// TestStopOwnedKillsOwnChild covers the accepted path with a throwaway child, so
// the proof (this shell is the parent) is exercised without touching anything
// outside this test.
func TestStopOwnedKillsOwnChild(t *testing.T) {
	child := startThrowawayChild(t)

	started, err := procinfo.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("StartTime(child): %v", err)
	}
	killed, err := StopOwned(Endpoint{}, child.Process.Pid, started.UnixNano())
	if err != nil {
		t.Fatalf("StopOwned(own child) = %v, want nil", err)
	}
	if len(killed) != 1 || killed[0] != child.Process.Pid {
		t.Fatalf("StopOwned(own child) killed %v, want [%d]", killed, child.Process.Pid)
	}
	waitChildGone(t, child)
}

// TestStopOwnedStopsProvenWrapperWhenListenerRecordIsStale is the regression test
// for --stop-on-exit leaving a self-started instance running: once the process
// serving the port has moved on (restart, or a record gone stale), the record's
// listener no longer proves anything -- and the old code then reported a
// successful stop without stopping anything. The wrapper is the durable proof,
// so its tree must still be terminated (and reported).
func TestStopOwnedStopsProvenWrapperWhenListenerRecordIsStale(t *testing.T) {
	child := startThrowawayChild(t)
	started, err := procinfo.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("StartTime(child): %v", err)
	}

	// A record whose listener half is garbage but whose wrapper half is the child
	// this process spawned -- exactly what a changed serving PID leaves behind.
	record := Endpoint{
		Host:              "127.0.0.1",
		Port:              freeListeningPort(t),
		ListenerPID:       999_999,
		ListenerStartedAt: 1,
		SpawnerPID:        child.Process.Pid,
		SpawnerStartedAt:  started.UnixNano(),
	}
	if record.Live(record.Host, record.Port) {
		t.Fatal("the fixture must not look live through its listener half")
	}
	if !record.Owned() {
		t.Fatal("the wrapper half of the record should still prove ownership")
	}

	killed, err := StopOwned(record, 0, 0)
	if err != nil {
		t.Fatalf("StopOwned = %v, want nil", err)
	}
	if len(killed) != 1 || killed[0] != child.Process.Pid {
		t.Fatalf("StopOwned killed %v, want the proven wrapper [%d]", killed, child.Process.Pid)
	}
	waitChildGone(t, child)
}

// TestStopOwnedProvesSessionWrapperWithoutRecord covers the in-session case: the
// wrapper this shell just spawned is a direct child, so it is stoppable even
// when the record is missing.
func TestStopOwnedProvesSessionWrapperWithoutRecord(t *testing.T) {
	child := startThrowawayChild(t)
	started, err := procinfo.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("StartTime(child): %v", err)
	}

	killed, err := StopOwned(Endpoint{Port: silentPort(t)}, child.Process.Pid, started.UnixNano())
	if err != nil || len(killed) != 1 || killed[0] != child.Process.Pid {
		t.Fatalf("StopOwned(session wrapper with no record) = (%v, %v), want the wrapper killed", killed, err)
	}
	waitChildGone(t, child)
}

// TestEndpointOwned pins the gate --stop-on-exit uses before it even tries.
func TestEndpointOwned(t *testing.T) {
	stale := time.Now().Add(-time.Hour).UnixNano()
	child := startThrowawayChild(t)
	started, err := procinfo.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("StartTime(child): %v", err)
	}

	if !(Endpoint{SpawnerPID: child.Process.Pid, SpawnerStartedAt: started.UnixNano()}).Owned() {
		t.Fatal("a live wrapper with a matching creation time is ours")
	}
	if !(Endpoint{SpawnerPID: child.Process.Pid}).Owned() {
		t.Fatal("a live wrapper that is this process's own child is ours")
	}
	if (Endpoint{SpawnerPID: 999_999, SpawnerStartedAt: started.UnixNano()}).Owned() {
		t.Fatal("a dead/unrelated wrapper is not ours")
	}
	if (Endpoint{}).Owned() {
		t.Fatal("an empty record is not ours")
	}
	if (Endpoint{Host: "127.0.0.1", Port: 3080, ListenerPID: 999_999, ListenerStartedAt: stale}).Owned() {
		t.Fatal("a dead listener and no wrapper is not ours")
	}
}
