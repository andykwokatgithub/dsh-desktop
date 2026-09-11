package service

import (
	"errors"
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

// TestStopOwnedRefusesUnprovenPID is the safety net behind --stop-on-exit: a PID
// that is neither this shell's child nor matched by a recorded creation time is
// never killed, so a recycled PID cannot take down an unrelated process.
func TestStopOwnedRefusesUnprovenPID(t *testing.T) {
	stale := time.Now().Add(-time.Hour).UnixNano()

	// A wrapper PID whose recorded creation time does not match, and which is not
	// our child, must be refused (this is the case a stale record produces).
	err := StopOwned(Endpoint{Host: "127.0.0.1", Port: 1, SpawnerPID: os.Getpid(), SpawnerStartedAt: stale}, 0, 0)
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("StopOwned with an unproven wrapper = %v, want ErrNotOwned", err)
	}
	// Same for a session spawner that cannot be proven this time.
	if err := StopOwned(Endpoint{}, os.Getpid(), stale); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("StopOwned with a stale session spawner = %v, want ErrNotOwned", err)
	}

	// A recorded listener that no longer matches the record is treated as gone:
	// nothing is killed and no error is raised. (If this wrongly killed by PID,
	// this test process would die and the test would fail loudly.)
	if err := StopOwned(Endpoint{Host: "127.0.0.1", Port: 1, ListenerPID: os.Getpid(), ListenerStartedAt: stale}, 0, 0); err != nil {
		t.Fatalf("StopOwned with a stale listener record = %v, want nil", err)
	}

	// Nothing recorded is not an error either.
	if err := StopOwned(Endpoint{}, 0, 0); err != nil {
		t.Fatalf("StopOwned with no PID = %v, want nil", err)
	}
}

// TestStopOwnedKillsOwnChild covers the accepted path with a throwaway child, so
// the proof (this shell is the parent) is exercised without touching anything
// outside this test.
func TestStopOwnedKillsOwnChild(t *testing.T) {
	child := exec.Command("cmd", "/c", "ping -n 30 127.0.0.1 >nul")
	if err := child.Start(); err != nil {
		t.Skipf("cannot start a throwaway child: %v", err)
	}
	defer func() {
		if child.Process != nil {
			_ = child.Process.Kill()
		}
	}()

	started, err := procinfo.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatalf("StartTime(child): %v", err)
	}
	if err := StopOwned(Endpoint{}, child.Process.Pid, started.UnixNano()); err != nil {
		t.Fatalf("StopOwned(own child) = %v, want nil", err)
	}

	// The process handle stays open here, so PID liveness is not a reliable exit
	// signal -- wait on the process itself instead.
	done := make(chan struct{})
	go func() {
		_ = child.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the child is still alive after StopOwned")
	}
}
