package config

import (
	"testing"
	"time"
)

func TestCanonicalURL(t *testing.T) {
	c := Default()
	if got := c.CanonicalURL(); got != "http://127.0.0.1:3080" {
		t.Fatalf("CanonicalURL() = %q, want %q", got, "http://127.0.0.1:3080")
	}

	c.Host = "localhost"
	c.Port = 4000
	if got := c.CanonicalURL(); got != "http://localhost:4000" {
		t.Fatalf("CanonicalURL() = %q, want %q", got, "http://localhost:4000")
	}
}

func TestPageURL(t *testing.T) {
	c := Default()
	// No override: derived from host:port.
	if got := c.PageURL(); got != c.CanonicalURL() {
		t.Fatalf("PageURL() = %q, want derived %q", got, c.CanonicalURL())
	}
	// Explicit override is honored verbatim.
	c.URL = "http://127.0.0.1:3080/ui"
	if got := c.PageURL(); got != "http://127.0.0.1:3080/ui" {
		t.Fatalf("PageURL() with override = %q, want %q", got, "http://127.0.0.1:3080/ui")
	}
}

func TestDurationMethods(t *testing.T) {
	c := Default()
	if got := c.StartupTimeoutDuration(); got != 30*time.Second {
		t.Fatalf("StartupTimeoutDuration() = %v, want %v", got, 30*time.Second)
	}
	if got := c.PollIntervalDuration(); got != 500*time.Millisecond {
		t.Fatalf("PollIntervalDuration() = %v, want %v", got, 500*time.Millisecond)
	}
}

func TestValidate(t *testing.T) {
	base := Default()
	c := base
	if err := c.validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Config)
		want   bool // true if we expect an error
	}{
		{"host empty", func(c *Config) { c.Host = "" }, true},
		{"host all interfaces", func(c *Config) { c.Host = "0.0.0.0" }, true},
		{"port 0", func(c *Config) { c.Port = 0 }, true},
		{"port over 65535", func(c *Config) { c.Port = 70000 }, true},
		{"width 0", func(c *Config) { c.WindowWidth = 0 }, true},
		{"height negative", func(c *Config) { c.WindowHeight = -1 }, true},
		{"startup-timeout 0", func(c *Config) { c.StartupTimeoutSec = 0 }, true},
		{"poll-ms 0", func(c *Config) { c.PollIntervalMS = 0 }, true},
		// --url lone: host and port are adopted from it.
		{"url derives host", func(c *Config) { c.URL = "http://localhost:3080" }, false},
		{"url derives port", func(c *Config) { c.URL = "http://127.0.0.1:4000" }, false},
		// --url against an explicit endpoint must agree with it.
		{"url host mismatch with --host", func(c *Config) { c.HostSet, c.URL = true, "http://localhost:3080" }, true},
		{"url port mismatch with --port", func(c *Config) { c.PortSet, c.URL = true, "http://127.0.0.1:4000" }, true},
		{"url explicit endpoint matches", func(c *Config) { c.HostSet, c.PortSet, c.URL = true, true, "http://127.0.0.1:3080/?token=x" }, false},
		{"url bad scheme", func(c *Config) { c.URL = "ftp://127.0.0.1:3080" }, true},
		{"url without port", func(c *Config) { c.URL = "http://127.0.0.1/" }, true},
		{"url without host", func(c *Config) { c.URL = "http://:3080/" }, true},
		{"url valid", func(c *Config) { c.URL = "http://127.0.0.1:3080" }, false},
		{"url https valid", func(c *Config) { c.URL = "https://127.0.0.1:3080" }, false},
		{"url with token", func(c *Config) { c.URL = "http://127.0.0.1:3080/?token=abc" }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			err := c.validate()
			if tc.want && err == nil {
				t.Fatalf("validate() = nil, want error (%s)", tc.name)
			}
			if !tc.want && err != nil {
				t.Fatalf("validate() = %v, want nil (%s)", err, tc.name)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	cfg, err := Parse([]string{"--port", "4000", "--url", "http://127.0.0.1:4000", "--title", "My Harness"})
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Port != 4000 {
		t.Fatalf("Port = %d, want 4000", cfg.Port)
	}
	if cfg.WindowTitle != "My Harness" {
		t.Fatalf("WindowTitle = %q, want %q", cfg.WindowTitle, "My Harness")
	}
	// URL override must match host:port after flags are applied.
	if cfg.URL != "http://127.0.0.1:4000" {
		t.Fatalf("URL = %q, want %q", cfg.URL, "http://127.0.0.1:4000")
	}

	// A mismatched --url must be rejected by Parse (validation runs after flags).
	if _, err := Parse([]string{"--port", "4000", "--url", "http://127.0.0.1:3080"}); err == nil {
		t.Fatal("Parse() with mismatched --url should error")
	}

	// Single-dash form stays accepted for backward compatibility.
	cfg, err = Parse([]string{"-port", "5000"})
	if err != nil {
		t.Fatalf("Parse() single-dash error: %v", err)
	}
	if cfg.Port != 5000 {
		t.Fatalf("Port = %d, want 5000", cfg.Port)
	}
	if !cfg.PortSet {
		t.Fatal("PortSet = false, want true when --port is written")
	}
}

// TestURLTokenAndDerivation covers FR-05: an explicit --url supplies the
// endpoint when --host/--port were left at their defaults, and it may carry the
// process launch token.
func TestURLTokenAndDerivation(t *testing.T) {
	cfg, err := Parse([]string{"--url", "http://127.0.0.1:34567/?token=abc-123"})
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 34567 {
		t.Fatalf("derived endpoint = %s:%d, want 127.0.0.1:34567", cfg.Host, cfg.Port)
	}
	if got := cfg.URLToken(); got != "abc-123" {
		t.Fatalf("URLToken() = %q, want %q", got, "abc-123")
	}
	if cfg.CanonicalURL() != "http://127.0.0.1:34567" {
		t.Fatalf("CanonicalURL() = %q", cfg.CanonicalURL())
	}
	if cfg.PageURL() != "http://127.0.0.1:34567/?token=abc-123" {
		t.Fatalf("PageURL() = %q, want the explicit URL verbatim", cfg.PageURL())
	}

	// A URL without a token yields "" and leaves the default endpoint alone.
	cfg, err = Parse([]string{"--url", "http://127.0.0.1:3080/"})
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := cfg.URLToken(); got != "" {
		t.Fatalf("URLToken() = %q, want empty", got)
	}

	// An explicit --host/--port still wins, and a contradicting --url fails.
	if _, err := Parse([]string{"--host", "127.0.0.1", "--port", "4000", "--url", "http://127.0.0.1:4000/?token=x"}); err != nil {
		t.Fatalf("consistent explicit endpoint + --url should parse: %v", err)
	}
	if _, err := Parse([]string{"--port", "4000", "--url", "http://127.0.0.1:4001/?token=x"}); err == nil {
		t.Fatal("Parse() with a contradicting --url port should error")
	}
}
