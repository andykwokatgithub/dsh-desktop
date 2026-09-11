// Package service manages the dsh web backend: health probing, spawning and
// lifecycle (persist-or-stop) semantics.
//
// Design notes (PRD V1.1):
//   - D4: readiness is a port + HTTP health check, not just a TCP dial. dsh web
//     now gates the UI behind browser auth, so a bare GET `/` answers with a
//     distinctive 401; the health check treats that dsh-web-auth response (and
//     200 / 303) as "up", still rejecting a foreign or stale port occupant.
//   - D5: readiness is confirmed by polling the health check, optionally
//     assisted by the `dsh web: <url>` line on the child's stdout (which also
//     carries the authenticated URL with the process launch token).
//   - D6: on Windows the global `dsh` is an npm .cmd/.ps1 shim, so it is
//     spawned through `cmd /c` so PATHEXT resolution applies, and the child
//     inherits PATH/DSH_HOME.
package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/deepseek-ai/dsh-desktop/internal/procinfo"
)

// Behavior captures the probe/poll/spawn parameters.
type Behavior struct {
	Host           string
	Port           int
	Command        string
	StartupTimeout time.Duration
	PollInterval   time.Duration
}

// Healthy reports whether a dsh web service is up and answering this origin.
//
// dsh web now gates its UI behind a per-process launch token (browser auth). A
// bare GET to `/` with no token and no browser cookie is answered by the auth
// boundary with a distinctive 401 — "dsh web authentication required; …" —
// instead of HTTP 200. That 401, like a 200 (valid cookie / auth off) or a 303
// (token->cookie exchange), proves the dsh web process is alive and responsive,
// so it counts as healthy. A port that is open but does not answer with a
// dsh-web signal still returns false, preserving the D4 distinction between a
// running dsh service and a foreign/stale occupant of the port.
func (b Behavior) Healthy() bool {
	u := fmt.Sprintf("http://%s:%d/", b.Host, b.Port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusSeeOther:
		// Served (cookie present / auth off) or token->cookie redirect.
		return true
	case http.StatusUnauthorized:
		// Confirm it is dsh web's auth boundary rather than an arbitrary 401:
		// the auth-fence body is "dsh web authentication required; …".
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return bytes.Contains(bytes.ToLower(body), []byte("dsh web"))
	}
	return false
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

// State classifies what answers on an endpoint.
type State int

const (
	// StateFree: nothing is listening.
	StateFree State = iota
	// StateHealthy: something answers with a dsh-web signal.
	StateHealthy
	// StateOccupied: something listens but shows no dsh-web signal.
	StateOccupied
)

// Probe classifies the endpoint. Identity -- is the occupant really dsh, and is
// it ours? -- is decided separately (procinfo for the process, Endpoint for the
// record); see the startup decision in main.go.
func (b Behavior) Probe() State {
	if b.Healthy() {
		return StateHealthy
	}
	if b.PortOpen() {
		return StateOccupied
	}
	return StateFree
}

// Cmd bundles an in-flight spawned dsh process, its stdout pipe, and the
// authenticated URL (with the process launch token) read from its ready line.
type Cmd struct {
	proc     *exec.Cmd
	stdout   io.ReadCloser
	mu       sync.Mutex
	readyURL string
}

// PID returns the process id of the spawned `cmd` wrapper.
func (c *Cmd) PID() int { return c.proc.Process.Pid }

// Stdout exposes the child's stdout, used to detect the `dsh web: <url>` line.
func (c *Cmd) Stdout() io.Reader { return c.stdout }

// setReadyURL records the authenticated URL from the child's ready line.
func (c *Cmd) setReadyURL(u string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if u != "" {
		c.readyURL = u
	}
}

// ReadyURL returns the authenticated URL (with the launch token) advertised on
// the child's `dsh web: <url>` ready line, or "" until that line is seen.
func (c *Cmd) ReadyURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readyURL
}

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
//
// When the service was spawned by the caller (c != nil) it returns the
// authenticated URL — the one with the process launch token — advertised on the
// `dsh web: <url>` ready line, so the caller can bootstrap the WebView browser
// cookie by navigating there. It returns "" when no Cmd was supplied (the reuse
// path, where the WebView's persisted browser cookie already authorizes it).
func WaitHealthy(ctx context.Context, b Behavior, c *Cmd) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, b.StartupTimeout)
	defer cancel()

	// Optionally watch the child stdout for a readiness announcement.
	readyLine := make(chan struct{}, 1)
	if c != nil {
		go watchReadyLine(c.stdout, c, readyLine)
	}

	ticker := time.NewTicker(b.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("service not healthy within %s: %w", b.StartupTimeout, ctx.Err())
		case <-readyLine:
			// A ready-adjacent line appeared; if the auth-aware health check also
			// passes we have the token URL to navigate the WebView to.
			if c != nil && c.ReadyURL() != "" && b.Healthy() {
				return c.ReadyURL(), nil
			}
		case <-ticker.C:
			if b.Healthy() {
				if c == nil {
					return "", nil
				}
				if u := c.ReadyURL(); u != "" {
					return u, nil
				}
				// Healthy but the dsh web: <url> line has not been printed yet.
				// Keep polling for it (printUrl is always on, so it arrives
				// shortly after the auth boundary comes up).
			}
		}
	}
}

// Stop terminates the whole dsh process tree (dsh + node worker/subagent). A
// process that has already exited is not an error, and a genuinely failed kill
// is reported so callers can say so instead of pretending it worked.
func Stop(pid int) error {
	if pid <= 0 {
		return nil
	}
	// taskkill /F /T kills the process and all of its descendants.
	if err := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run(); err == nil {
		return nil
	}
	// Fall back to the actual pid if the wrapper already exited.
	if err := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run(); err == nil {
		return nil
	}
	if _, err := procinfo.StartTime(pid); err != nil {
		return nil // the process is gone; nothing to report
	}
	return fmt.Errorf("service: taskkill %d failed", pid)
}

func watchReadyLine(r io.Reader, c *Cmd, ch chan<- struct{}) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "dsh web") {
			if c != nil {
				c.setReadyURL(urlFromReadyLine(line))
			}
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
}

// urlFromReadyLine extracts the authenticated URL from the child's ready line,
// e.g. "dsh web: http://127.0.0.1:3080/?token=… (LAN: http://…)". It returns the
// URL (carrying the process launch token) or "" when the line is not in the
// expected form.
func urlFromReadyLine(line string) string {
	s := strings.TrimPrefix(line, "dsh web")
	s = strings.TrimLeft(s, " :")
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	if !strings.HasPrefix(s, "http://") {
		return ""
	}
	return s
}
