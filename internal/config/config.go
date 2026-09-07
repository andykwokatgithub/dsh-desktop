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
//	go build -ldflags "-X github.com/deepseek-ai/dsh-desktop/internal/config.Version=0.2.7" .
var Version = "0.2.9"

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
	WindowTitle       string
	ShowVersion       bool // print the version and exit
	CheckUpdate       bool // check GitHub Releases for a newer version and exit
	Update            bool // download + apply a newer version and exit

	// URL is an optional explicit override for the page the shell loads. It is
	// empty by default, in which case the page address is derived from Host+Port
	// (see CanonicalURL). When set it is used verbatim as the navigation target,
	// but only after validate() confirms it points at the same host:port that is
	// probed/spawned, so the page and the service can never diverge.
	URL string
}

// Default returns the recommended defaults (see PRD V1.1).
func Default() Config {
	return Config{
		Host:              "127.0.0.1",
		Port:              3080,
		Command:           "web",
		StopOnExit:        false, // D1: persist the service across window closes
		DevTools:          false, // D7: safe default
		ContextMenu:       true, // leave the WebView2 context menu enabled by default (opt-out via --context-menu=false)
		StartupTimeoutSec: 30,
		PollIntervalMS:    500,
		WindowWidth:       1200,
		WindowHeight:      800,
		WindowTitle:       "DeepSeek Harness",
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
	fs.BoolVar(&cfg.StopOnExit, "stop-on-exit", cfg.StopOnExit, "stop the dsh service when the window closes")
	fs.BoolVar(&cfg.DevTools, "devtools", cfg.DevTools, "enable WebView2 developer tools")
	fs.BoolVar(&cfg.ContextMenu, "context-menu", cfg.ContextMenu, "leave the WebView2 default context menu enabled")
	fs.IntVar(&cfg.StartupTimeoutSec, "startup-timeout", cfg.StartupTimeoutSec, "seconds to wait for the service to become healthy")
	fs.IntVar(&cfg.PollIntervalMS, "poll-ms", cfg.PollIntervalMS, "health-check polling interval (ms)")
	fs.IntVar(&cfg.WindowWidth, "width", cfg.WindowWidth, "window width")
	fs.IntVar(&cfg.WindowHeight, "height", cfg.WindowHeight, "window height")
	fs.StringVar(&cfg.WindowTitle, "title", cfg.WindowTitle, "window title")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "print version and exit")
	fs.BoolVar(&cfg.CheckUpdate, "check-update", false, "check GitHub Releases for a newer version and exit")
	fs.BoolVar(&cfg.Update, "update", false, "download and apply the latest release, then exit")
	fs.StringVar(&cfg.URL, "url", cfg.URL, "canonical URL of the dsh web UI (optional override; must match --host/--port)")

	// Custom usage so the documented double-dash style is what --help prints.
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "用法: dsh-desktop [选项]\n\n选项:\n")
		fs.VisitAll(func(f *flag.Flag) {
			name := "--" + f.Name
			def := ""
			if f.DefValue != "" && f.DefValue != "false" {
				def = "  (默认 " + f.DefValue + ")"
			}
			fmt.Fprintf(out, "  %-24s  %s%s\n", name, f.Usage, def)
		})
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validate checks the semantic bounds of the parsed parameters so a
// misconfigured invocation fails loudly instead of misbehaving at runtime.
func (c *Config) validate() error {
	if c.Host == "" {
		return fmt.Errorf("参数 --host 不能为空")
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
	if c.URL != "" {
		return c.checkURL()
	}
	return nil
}

// checkURL ensures an explicit --url still points at the endpoint the shell
// probes/spawns, so navigation and service checks never diverge.
func (c *Config) checkURL() error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("参数 --url 无效: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("参数 --url 的 scheme 必须是 http/https, 当前为 %q", u.Scheme)
	}
	if !strings.EqualFold(u.Hostname(), c.Host) {
		return fmt.Errorf("参数 --url(%q) 的主机 %q 与 --host(%q) 不一致", c.URL, u.Hostname(), c.Host)
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	if port != strconv.Itoa(c.Port) {
		return fmt.Errorf("参数 --url(%q) 的端口 %q 与 --port(%d) 不一致", c.URL, port, c.Port)
	}
	return nil
}
