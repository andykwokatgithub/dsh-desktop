// Package config holds the runtime options for the dsh-desktop shell.
package config

import (
	"flag"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the application version. It can be overridden at build time:
//
//	go build -ldflags "-X github.com/deepseek-ai/dsh-desktop/internal/config.Version=0.3.0" .
var Version = "0.3.0"

// TitleVersion is the version token shown in the window title: the bare version
// ("0.3.0"), matching what --version prints.
func TitleVersion() string {
	return Version
}

// TitleVersionPlaceholder is the placeholder --title may contain; it is expanded
// to TitleVersion(). The default window title ends with it, so the window title
// bar always shows the running version.
const TitleVersionPlaceholder = "{version}"

// DefaultWindowTitleBase is the product part of the default title.
const DefaultWindowTitleBase = "DeepSeek Harness Desktop"

// DefaultWindowTitle is the default --title template: the window title bar (and
// taskbar entry) shows the running version, which is what users quote in bug
// reports and screenshots.
func DefaultWindowTitle() string {
	return DefaultWindowTitleBase + " " + TitleVersionPlaceholder
}

// Config is the fully-resolved runtime configuration.
type Config struct {
	Host              string // bind host used by the health probe and dsh spawn
	Port              int    // listen port (dsh web default 3080)
	Command           string // dsh subcommand (web -> --profile web alias)
	StopOnExit        bool   // whether closing the window also stops the dsh service
	DevTools          bool   // expose WebView2 developer tools
	ContextMenu       bool   // leave the WebView2 default context menu enabled
	StartupTimeoutSec int    // seconds to wait for the service to become healthy
	PollIntervalMS    int    // health-check polling interval (ms)
	WindowWidth       int
	WindowHeight      int
	WindowTitle       string // raw --title template (may contain {version})
	ShowVersion       bool   // print the version and exit
	CheckUpdate       bool   // check GitHub Releases for a newer version and exit
	Update            bool   // download + apply a newer version and exit

	// URL is an optional explicit target: the dsh web URL, optionally carrying
	// the process launch token ("http://127.0.0.1:3080/?token=..."). Its host and
	// port are adopted as Host/Port (see applyURL) unless --host/--port were
	// given explicitly, in which case a mismatch is an error, so the page
	// address the shell loads and the endpoint it probes can never diverge.
	URL string

	// HostSet/PortSet record whether --host/--port were given explicitly, which
	// decides whether an explicit --url may derive them.
	HostSet bool
	PortSet bool
}

// URLToken returns the token query parameter of an explicit --url, or "".
func (c *Config) URLToken() string {
	if c.URL == "" {
		return ""
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return ""
	}
	return u.Query().Get("token")
}

// Default returns the recommended defaults (see PRD V1.1).
func Default() Config {
	return Config{
		Host:              "127.0.0.1",
		Port:              3080,
		Command:           "web",
		StopOnExit:        false, // D1: persist the service across window closes
		DevTools:          false, // D7: safe default
		ContextMenu:       true,  // leave the WebView2 context menu enabled by default (opt-out via --context-menu=false)
		StartupTimeoutSec: 30,
		PollIntervalMS:    500,
		WindowWidth:       1200,
		WindowHeight:      800,
		WindowTitle:       DefaultWindowTitle(),
	}
}

// CanonicalURL is the dsh web origin the shell targets, derived from Host+Port
// so the page address always tracks the endpoint that is probed and spawned.
func (c *Config) CanonicalURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.Port)
}

// PageURL is the address the shell actually loads. It honors an explicit --url
// override verbatim; otherwise it falls back to the canonical host:port URL.
func (c *Config) PageURL() string {
	if c.URL != "" {
		return c.URL
	}
	return c.CanonicalURL()
}

// WindowTitleText resolves the raw --title template into the string the window
// title bar actually shows: every "{version}" placeholder becomes the running
// version. The default template ends with it, so "out of the box" the title
// reads "DeepSeek Harness Desktop 0.3.0" and users can quote the version they run.
//
// A --title without the placeholder is honored verbatim (the user asked for that
// exact title), and an empty/blank --title falls back to the default base plus
// the version rather than leaving the window without a caption.
func (c *Config) WindowTitleText() string {
	base := strings.TrimSpace(c.WindowTitle)
	if base == "" {
		return DefaultWindowTitleBase + " " + TitleVersion()
	}
	if !strings.Contains(base, TitleVersionPlaceholder) {
		return base
	}
	// Collapse the whitespace a template like "Harness {version} " would leave.
	return strings.TrimSpace(strings.ReplaceAll(base, TitleVersionPlaceholder, TitleVersion()))
}

// StartupTimeoutDuration converts the seconds flag into a time.Duration.
func (c *Config) StartupTimeoutDuration() time.Duration {
	return time.Duration(c.StartupTimeoutSec) * time.Second
}

// PollIntervalDuration converts the milliseconds flag into a time.Duration.
func (c *Config) PollIntervalDuration() time.Duration {
	return time.Duration(c.PollIntervalMS) * time.Millisecond
}

// Parse builds a Config from command-line flags, applying defaults first.
func Parse(args []string) (*Config, error) {
	cfg := Default()
	fs := flag.NewFlagSet("dsh-desktop", flag.ContinueOnError)
	fs.StringVar(&cfg.Host, "host", cfg.Host, "bind host used by the health probe and dsh spawn")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port of the dsh web service")
	fs.StringVar(&cfg.Command, "command", cfg.Command, "dsh subcommand to spawn (web)")
	fs.BoolVar(&cfg.StopOnExit, "stop-on-exit", cfg.StopOnExit, "stop the dsh service this shell started when the window closes")
	fs.BoolVar(&cfg.DevTools, "devtools", cfg.DevTools, "enable WebView2 developer tools")
	fs.BoolVar(&cfg.ContextMenu, "context-menu", cfg.ContextMenu, "leave the WebView2 default context menu enabled")
	fs.IntVar(&cfg.StartupTimeoutSec, "startup-timeout", cfg.StartupTimeoutSec, "seconds to wait for the service to become healthy")
	fs.IntVar(&cfg.PollIntervalMS, "poll-ms", cfg.PollIntervalMS, "health-check polling interval (ms)")
	fs.IntVar(&cfg.WindowWidth, "width", cfg.WindowWidth, "window width")
	fs.IntVar(&cfg.WindowHeight, "height", cfg.WindowHeight, "window height")
	fs.StringVar(&cfg.WindowTitle, "title", cfg.WindowTitle, "window title ({version} expands to the running version)")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "print version and exit")
	fs.BoolVar(&cfg.CheckUpdate, "check-update", false, "check GitHub Releases for a newer version and exit")
	fs.BoolVar(&cfg.Update, "update", false, "download and apply the latest release, then exit")
	fs.StringVar(&cfg.URL, "url", cfg.URL, "dsh web URL to attach to (may carry ?token=…; its host/port are adopted)")

	// Custom usage so the documented double-dash style is what --help prints.
	// The output is shown in Chinese or English depending on the current system
	// language (see useChineseUI).
	fs.Usage = func() {
		writeUsage(fs, useChineseUI())
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	// Remember which endpoint flags were written explicitly: an explicit --url
	// may derive Host/Port only when they were left at their defaults.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			cfg.HostSet = true
		case "port":
			cfg.PortSet = true
		}
	})
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validate checks the semantic bounds of the parsed parameters so a
// misconfigured invocation fails loudly instead of misbehaving at runtime. An
// explicit --url is resolved first, because it may supply Host/Port.
func (c *Config) validate() error {
	if c.URL != "" {
		if err := c.applyURL(); err != nil {
			return err
		}
	}
	if c.Host == "" {
		return fmt.Errorf("参数 --host 不能为空")
	}
	if c.Host == "0.0.0.0" {
		return fmt.Errorf("参数 --host 不支持 0.0.0.0（dsh 顶层已拒绝该绑定；请用 127.0.0.1）")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("参数 --port 必须在 1-65535 之间, 当前为 %d", c.Port)
	}
	if c.WindowWidth <= 0 {
		return fmt.Errorf("参数 --width 必须大于 0, 当前为 %d", c.WindowWidth)
	}
	if c.WindowHeight <= 0 {
		return fmt.Errorf("参数 --height 必须大于 0, 当前为 %d", c.WindowHeight)
	}
	if c.StartupTimeoutSec <= 0 {
		return fmt.Errorf("参数 --startup-timeout 必须大于 0, 当前为 %d", c.StartupTimeoutSec)
	}
	if c.PollIntervalMS <= 0 {
		return fmt.Errorf("参数 --poll-ms 必须大于 0, 当前为 %d", c.PollIntervalMS)
	}
	return nil
}

// applyURL resolves an explicit --url.
//
// An explicit --host/--port wins: a --url that contradicts either is a mistake
// worth failing on. When they were left at their defaults, the URL's host and
// port are adopted, so "dsh-desktop --url http://127.0.0.1:34567/?token=…" can
// attach to an instance that is not on the preferred port. A port-less URL is
// refused: it would silently mean :80 while the shell probes 3080.
func (c *Config) applyURL() error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("参数 --url 无效: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("参数 --url 的 scheme 必须是 http/https, 当前为 %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("参数 --url 缺少主机名: %q", c.URL)
	}
	portText := u.Port()
	if portText == "" {
		return fmt.Errorf("参数 --url 必须包含显式端口（例如 http://127.0.0.1:3080/?token=…）: %q", c.URL)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("参数 --url 的端口无效: %q", portText)
	}
	if c.PortSet && port != c.Port {
		return fmt.Errorf("参数 --url(%q) 的端口 %d 与 --port(%d) 不一致", c.URL, port, c.Port)
	}
	if c.HostSet && !strings.EqualFold(u.Hostname(), c.Host) {
		return fmt.Errorf("参数 --url(%q) 的主机 %q 与 --host(%q) 不一致", c.URL, u.Hostname(), c.Host)
	}
	c.Host = u.Hostname()
	c.Port = port
	return nil
}
