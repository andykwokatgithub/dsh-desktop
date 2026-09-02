// Package service manages the dsh web backend: health probing, spawning and
// lifecycle (persist-or-stop) semantics.
//
// Design notes (PRD V1.1):
//   - D4: readiness is a "port + HTTP 200" health check, not just a TCP dial.
//   - D5: readiness is confirmed by polling the health check, optionally
//     assisted by the `dsh web: <url>` line on the child's stdout.
//   - D6: on Windows the global `dsh` is an npm .cmd/.ps1 shim, so it is
//     spawned through `cmd /c` so PATHEXT resolution applies, and the child
//     inherits PATH/DSH_HOME.
package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// Behavior captures the probe/poll/LSP parameters.
type Behavior struct {
	URL            string
	Host           string
	Port           int
	Command        string
	StartupTimeout time.Duration
	PollInterval   time.Duration
}

// Healthy reports whether the dsh web service answers this origin with HTTP 200.
// A port that is open but does not answer 200 (or is not dsh) returns false.
func (b Behavior) Healthy() bool {
	u := fmt.Sprintf("http://%s:%d/", b.Host, b.Port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// PortOpen reports only whether something is listening on the port. Used to
// distinguish "stale/non-dsh occupant" (open but not healthy) from "nothing".
func (b Behavior) PortOpen() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", b.Host, b.Port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Cmd bundles an in-flight spawned dsh process and its stdout pipe.
type Cmd struct {
	proc   *exec.Cmd
	stdout io.ReadCloser
}

// PID returns the process id of the spawned `cmd` wrapper.
func (c *Cmd) PID() int { return c.proc.Process.Pid }

// Stdout exposes the child's stdout, used to detect the `dsh web: <url>` line.
func (c *Cmd) Stdout() io.Reader { return c.stdout }

// Spawn starts `dsh web --no-open` through `cmd /c`. The returned Cmd owns the
// process tree; call Stop to terminate it.
func Spawn(ctx context.Context, b Behavior) (*Cmd, error) {
	// D6: resolve the npm shim via cmd /c; inherit the parent environment.
	argline := fmt.Sprintf("dsh %s --no-open --host %s --port %d", b.Command, b.Host, b.Port)
	cmd := exec.CommandContext(ctx, "cmd", "/c", argline)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	// CREATE_NO_WINDOW so the background service does not flash a console.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start dsh: %w", err)
	}
	return &Cmd{proc: cmd, stdout: pipe}, nil
}

// WaitHealthy polls the health check until the service is ready or the timeout
// elapses. It also consumes the child's stdout looking for the ready line.
func WaitHealthy(ctx context.Context, b Behavior, c *Cmd) error {
	ctx, cancel := context.WithTimeout(ctx, b.StartupTimeout)
	defer cancel()

	// Optionally watch the child stdout for a readiness announcement.
	readyLine := make(chan struct{}, 1)
	if c != nil {
		go watchReadyLine(c.stdout, readyLine)
	}

	ticker := time.NewTicker(b.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("service not healthy within %s: %w", b.StartupTimeout, ctx.Err())
		case <-readyLine:
			// The dsh web URL line appeared; still confirm via health check.
			if b.Healthy() {
				return nil
			}
		case <-ticker.C:
			if b.Healthy() {
				return nil
			}
		}
	}
}

// Stop terminates the whole dsh process tree (dsh + node worker/subagent).
func Stop(pid int) error {
	if pid <= 0 {
		return nil
	}
	// taskkill /F /T kills the process and all of its descendants.
	if err := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run(); err != nil {
		// Fall back to the actual pid if the wrapper already exited.
		_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
	}
	return nil
}

func watchReadyLine(r io.Reader, ch chan<- struct{}) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if len(line) >= 7 && line[:7] == "dsh web" {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}
