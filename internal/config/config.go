// Package config holds the runtime options for the dsh-desktop shell.
package config

import "flag"

// Config is the fully-resolved runtime configuration.
type Config struct {
	URL            string // canonical local URL served by dsh web
	Host           string // bind host for the health probe and dsh spawn
	Port           int    // listen port (dsh web default 3080)
	Command        string // dsh subcommand (web -> --profile web alias)
	StopOnExit     bool   // whether closing the window also stops the dsh service
	DevTools       bool   // expose WebView2 developer tools
	ContextMenu    bool   // leave the WebView2 default context menu enabled
	StartupTimeout int    // seconds to wait for the service to become healthy
	PollIntervalMS int    // health-check polling interval
	WindowWidth    int
	WindowHeight   int
	WindowTitle    string
}

// Default returns the recommended defaults (see PRD V1.1).
func Default() Config {
	return Config{
		URL:            "http://127.0.0.1:3080",
		Host:           "127.0.0.1",
		Port:           3080,
		Command:        "web",
		StopOnExit:     false, // D1: persist the service across window closes
		DevTools:       false, // D7: safe default
		ContextMenu:    false, // D7: safe default
		StartupTimeout: 30,
		PollIntervalMS: 500,
		WindowWidth:    1200,
		WindowHeight:   800,
		WindowTitle:    "DeepSeek Harness",
	}
}

// Parse builds a Config from command-line flags, applying defaults first.
func Parse(args []string) (*Config, error) {
	cfg := Default()
	fs := flag.NewFlagSet("dsh-desktop", flag.ContinueOnError)
	fs.StringVar(&cfg.URL, "url", cfg.URL, "canonical local URL of the dsh web UI")
	fs.StringVar(&cfg.Host, "host", cfg.Host, "bind host used by the health probe and dsh spawn")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port of the dsh web service")
	fs.StringVar(&cfg.Command, "command", cfg.Command, "dsh subcommand to spawn (web)")
	fs.BoolVar(&cfg.StopOnExit, "stop-on-exit", cfg.StopOnExit, "stop the dsh service when the window closes")
	fs.BoolVar(&cfg.DevTools, "devtools", cfg.DevTools, "enable WebView2 developer tools")
	fs.BoolVar(&cfg.ContextMenu, "context-menu", cfg.ContextMenu, "leave the WebView2 default context menu enabled")
	fs.IntVar(&cfg.StartupTimeout, "startup-timeout", cfg.StartupTimeout, "seconds to wait for the service to become healthy")
	fs.IntVar(&cfg.PollIntervalMS, "poll-ms", cfg.PollIntervalMS, "health-check polling interval (ms)")
	fs.IntVar(&cfg.WindowWidth, "width", cfg.WindowWidth, "window width")
	fs.IntVar(&cfg.WindowHeight, "height", cfg.WindowHeight, "window height")
	fs.StringVar(&cfg.WindowTitle, "title", cfg.WindowTitle, "window title")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &cfg, nil
}
