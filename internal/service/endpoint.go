package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepseek-ai/dsh-desktop/internal/procinfo"
)

// StartTimeTolerance absorbs the 100 ns Filetime granularity when comparing a
// recorded process start time with the live one (see procinfo.Info).
const StartTimeTolerance = time.Second

// Endpoint records the dsh web instance this shell started, so a later launch
// can tell "my instance" from "somebody else's instance on the same port"
// without guessing from browser cookies.
//
// ListenerPID is the process that actually owns the listening socket (on a real
// install: node.exe), while SpawnerPID is the cmd.exe wrapper this shell
// spawned, which is the handle used for tree-kill.
type Endpoint struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	ListenerPID       int    `json:"listenerPid"`
	ListenerStartedAt int64  `json:"listenerStartedAt"` // Unix nanoseconds
	SpawnerPID        int    `json:"spawnerPid"`
	SpawnerStartedAt  int64  `json:"spawnerStartedAt"` // Unix nanoseconds, verified before any kill
}

// LoadEndpoint reads the record. A missing, unreadable or malformed file is not
// an error: it simply means "no record", and callers re-decide from scratch.
func LoadEndpoint(path string) (Endpoint, bool) {
	if path == "" {
		return Endpoint{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Endpoint{}, false
	}
	var e Endpoint
	if err := json.Unmarshal(raw, &e); err != nil || e.Port <= 0 {
		return Endpoint{}, false
	}
	return e, true
}

// SaveEndpoint writes the record atomically (temp file + rename), so a crash
// mid-write can never leave a half-parsed record behind.
func SaveEndpoint(path string, e Endpoint) error {
	if path == "" {
		return nil
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ClearEndpoint removes the record; a missing file is not an error.
func ClearEndpoint(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Live reports whether the recorded instance is still the process behind
// host:port: same endpoint, same listener PID, and a creation time that matches
// the record (which is what makes the check survive PID reuse).
func (e Endpoint) Live(host string, port int) bool {
	if e.Port != port || e.ListenerPID <= 0 || e.ListenerStartedAt == 0 {
		return false
	}
	if e.Host != "" && host != "" && !strings.EqualFold(e.Host, host) {
		return false
	}
	return processAliveAt(e.ListenerPID, e.ListenerStartedAt)
}

// ListenerMemory resolves the identity of the process currently listening on
// host:port, for the record written right after a successful spawn. It returns
// zeroes when the owner cannot be read, which makes the record non-authoritative
// rather than wrong.
func ListenerMemory(host string, port int) (pid int, startedAt int64) {
	owner, err := procinfo.ListenerPID(host, port)
	if err != nil || owner <= 0 {
		return 0, 0
	}
	started, err := procinfo.StartTime(owner)
	if err != nil {
		return owner, 0
	}
	return owner, started.UnixNano()
}

// processAliveAt reports whether pid exists and was created at startedAt (Unix
// nanoseconds, within StartTimeTolerance).
func processAliveAt(pid int, startedAt int64) bool {
	got, err := procinfo.StartTime(pid)
	if err != nil {
		return false
	}
	diff := got.UnixNano() - startedAt
	if diff < 0 {
		diff = -diff
	}
	return diff <= int64(StartTimeTolerance)
}

// ErrNotOwned is returned when a stop request is refused because the PID cannot
// be proven to belong to a process this shell started.
var ErrNotOwned = errors.New("service: refusing to stop a process this shell cannot prove it started")

// StopOwned stops one of this shell's own dsh web instances and never anything
// else -- in particular, an instance the user started themselves is left alone
// even when --stop-on-exit is set.
//
// Proof is required before killing, because a PID only means something while the
// process it named is alive and Windows recycles PIDs: killing a stale PID could
// take down an unrelated process.
//
//   - A recorded listener is the only authority for its PID: when the record
//     still matches it (PID + creation time) it is stopped, and when it does not
//     the PID is treated as gone and nothing is killed.
//   - With no recorded listener (the service never became ready, or its owner
//     could not be read at spawn time) the cmd.exe wrapper is used instead, and
//     it must be provable: a direct child of this shell, or still the process
//     whose creation time the record captured.
func StopOwned(record Endpoint, sessionSpawnerPID int, sessionSpawnerStartedAt int64) error {
	if record.ListenerPID > 0 {
		if record.Live(record.Host, record.Port) {
			return Stop(record.ListenerPID)
		}
		return nil
	}

	pid, startedAt := record.SpawnerPID, record.SpawnerStartedAt
	if sessionSpawnerPID > 0 {
		pid, startedAt = sessionSpawnerPID, sessionSpawnerStartedAt
	}
	if pid <= 0 {
		return nil
	}
	if !procinfo.IsChildOf(pid, os.Getpid()) && !processAliveAt(pid, startedAt) {
		return ErrNotOwned
	}
	return Stop(pid)
}
