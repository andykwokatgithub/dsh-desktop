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

// Owned reports whether the record still identifies an instance this shell
// started, which is the gate --stop-on-exit checks before it tries to stop
// anything.
//
// The wrapper is the durable half of the record and the listener the volatile
// one: the cmd.exe wrapper this shell spawned stays alive for the whole life of
// the instance, while the process that serves the port can be restarted (with a
// new PID) inside the same tree. So a record is ownable when either is provable
// -- the wrapper is a direct child of this process or is still alive at the
// creation time recorded beside it, or the listener is still alive at its
// recorded creation time.
func (e Endpoint) Owned() bool {
	if processAliveAt(e.SpawnerPID, e.SpawnerStartedAt) || procinfo.IsChildOf(e.SpawnerPID, os.Getpid()) {
		return true
	}
	return e.Live(e.Host, e.Port)
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

// processAliveAt reports whether pid is a running process created at startedAt
// (Unix nanoseconds, within StartTimeTolerance).
//
// The creation time is the anti-PID-reuse guard; the exit check is the
// anti-zombie guard (a process object with an open handle can outlive its
// process -- see procinfo.Exited).
func processAliveAt(pid int, startedAt int64) bool {
	if pid <= 0 || startedAt == 0 {
		return false
	}
	got, err := procinfo.StartTime(pid)
	if err != nil {
		return false
	}
	diff := got.UnixNano() - startedAt
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(StartTimeTolerance) {
		return false
	}
	exited, err := procinfo.Exited(pid)
	if err != nil {
		return false // unreadable is not proof of ownership
	}
	return !exited
}

// ErrNotOwned is returned when a stop request is refused because the endpoint is
// still served but no process behind it can be proven to be one this shell
// started.
var ErrNotOwned = errors.New("service: refusing to stop a process this shell cannot prove it started")

// StopOwned stops dsh web instances this shell started and never anything else --
// in particular, an instance the user started themselves is left alone even when
// --stop-on-exit is set.
//
// Ownership is a lineage, not one PID. The primary target is therefore the
// cmd.exe wrapper this shell spawned (spawnerPid): it stays alive for the whole
// life of the instance, and killing its tree (/T) takes down whatever serves the
// endpoint even when the serving process changed PID since the record was
// written. The recorded listener is a second target, for an instance whose
// wrapper is already gone.
//
// Proof is required before any kill, because a PID only means something while the
// process it named is alive and Windows recycles PIDs:
//
//   - The wrapper is proven when it is a direct child of this shell (an instance
//     spawned in this session) or when it is still running at the creation time
//     recorded next to it (an instance spawned by an earlier run of this shell).
//   - The listener is proven when it is still running at its recorded creation
//     time.
//
// It returns the PIDs whose trees were terminated. A live endpoint with no
// provable owner yields ErrNotOwned -- the caller says so instead of claiming a
// stop that never happened -- and an instance that is already gone yields no PIDs
// and no error.
func StopOwned(record Endpoint, sessionSpawnerPID int, sessionSpawnerStartedAt int64) ([]int, error) {
	spawnerPID, spawnerStartedAt := record.SpawnerPID, record.SpawnerStartedAt
	if sessionSpawnerPID > 0 {
		spawnerPID, spawnerStartedAt = sessionSpawnerPID, sessionSpawnerStartedAt
	}

	provenSpawner := 0
	if spawnerPID > 0 && (procinfo.IsChildOf(spawnerPID, os.Getpid()) || processAliveAt(spawnerPID, spawnerStartedAt)) {
		provenSpawner = spawnerPID
	}

	var targets []int
	add := func(pid int) {
		if pid <= 0 {
			return
		}
		// Only a process that is still running is a stop target: a PID that has
		// already exited (its object can outlive it while a handle is open) would
		// otherwise be reported as "terminated" without anything being stopped.
		if exited, err := procinfo.Exited(pid); err != nil || exited {
			return
		}
		for _, t := range targets {
			if t == pid {
				return
			}
		}
		targets = append(targets, pid)
	}
	add(provenSpawner)
	if record.ListenerPID > 0 && record.Live(record.Host, record.Port) {
		add(record.ListenerPID)
	}
	// Whatever serves the recorded endpoint right now, when it lives inside the
	// tree we proved: a server that restarted under our own wrapper has a new PID
	// that the record cannot know about.
	if provenSpawner > 0 && record.Port > 0 {
		if owner, err := procinfo.ListenerPID(record.Host, record.Port); err == nil && owner > 0 {
			if procinfo.IsDescendantOf(owner, provenSpawner) {
				add(owner)
			}
		}
	}

	if len(targets) == 0 {
		// No proof: refuse when something still answers on the recorded endpoint,
		// and report "already gone" when nothing does.
		if record.Port > 0 {
			if owner, err := procinfo.ListenerPID(record.Host, record.Port); err == nil && owner > 0 {
				return nil, ErrNotOwned
			}
		}
		return nil, nil
	}

	var firstErr error
	for _, pid := range targets {
		if err := Stop(pid); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return targets, firstErr
}
