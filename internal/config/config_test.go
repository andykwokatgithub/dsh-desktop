package config

import (
	"errors"
	"flag"
	"strings"
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
	// An explicit --title without the placeholder is shown verbatim.
	if got := cfg.WindowTitleText(); got != "My Harness" {
		t.Fatalf("WindowTitleText() = %q, want %q", got, "My Harness")
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

// TestUnknownFlagSuggestion covers the typo hint on a mistyped flag: the flag
// package reports "flag provided but not defined" without saying which flag was
// meant, so Parse adds the closest defined name when it is close enough.
func TestUnknownFlagSuggestion(t *testing.T) {
	// The reported case: "-updaetg" should point at --update.
	_, err := Parse([]string{"-updaetg"})
	if err == nil {
		t.Fatal("Parse() with an unknown flag should error")
	}
	if !strings.Contains(err.Error(), "--update") {
		t.Fatalf("error %q should suggest --update", err.Error())
	}
	// The flag package's own English line must not survive: the console shows one
	// localized reason, not that reason twice.
	if strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("error %q should not echo the flag package's raw message", err.Error())
	}

	// "--help11" must land on --help, which the flag package answers itself and
	// therefore never exposes through VisitAll.
	_, err = Parse([]string{"--help11"})
	if err == nil {
		t.Fatal("Parse() with an unknown flag should error")
	}
	if !strings.Contains(err.Error(), "--help") {
		t.Fatalf("error %q should suggest --help", err.Error())
	}
	// The real --help still works and stays flag.ErrHelp.
	if _, err := Parse([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("Parse(--help) error = %v, want flag.ErrHelp", err)
	}

	// A name nowhere near any defined flag gets no hint.
	_, err = Parse([]string{"--zzzzzzzz"})
	if err == nil {
		t.Fatal("Parse() with an unknown flag should error")
	}
	if strings.Contains(err.Error(), "想用") || strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error %q should not suggest anything", err.Error())
	}
}

// TestParseErrorMessageLanguage pins that every parameter-error shape follows the
// system language (the same rule --help and the --update output already follow),
// keeping the flag and value detail in the user's language.
func TestParseErrorMessageLanguage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		zh   string
		en   string
	}{
		{"unknown flag", []string{"--updaetg"}, "未知参数 --updaetg（是否想用 --update？）", `unknown flag --updaetg (did you mean --update?)`},
		{"int value", []string{"--port", "abc"}, `参数 --port 的取值 "abc" 无效`, `invalid value "abc" for flag --port`},
		{"bool value", []string{"--devtools=maybe"}, `参数 --devtools 的布尔取值 "maybe" 无效（应为 true/false）`, `invalid boolean value "maybe" for flag --devtools (want true/false)`},
		{"missing value", []string{"--port"}, "参数 --port 缺少取值", "flag --port needs an argument"},
		{"bad syntax", []string{"---x"}, "参数写法有误: ---x", "bad flag syntax: ---x"},
		{"port range", []string{"--port", "70000"}, "参数 --port 必须在 1-65535 之间, 当前为 70000", "--port must be between 1 and 65535, got 70000"},
		{"url conflict", []string{"--port", "4000", "--url", "http://127.0.0.1:4001/"}, `参数 --url("http://127.0.0.1:4001/") 的端口 4001 与 --port(4000) 不一致`, `--url ("http://127.0.0.1:4001/") port 4001 disagrees with --port (4000)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LC_ALL", "zh_CN.UTF-8")
			_, err := Parse(tc.args)
			if err == nil {
				t.Fatalf("Parse(%q) should error", tc.args)
			}
			if err.Error() != tc.zh {
				t.Fatalf("zh error = %q, want %q", err.Error(), tc.zh)
			}
			t.Setenv("LC_ALL", "en_US.UTF-8")
			_, err = Parse(tc.args)
			if err == nil {
				t.Fatalf("Parse(%q) should error", tc.args)
			}
			if err.Error() != tc.en {
				t.Fatalf("en error = %q, want %q", err.Error(), tc.en)
			}
		})
	}
}

// TestUsageLocalized checks the help text a parameter error is followed by (and
// that --help prints): it must exist in both languages and list the flags.
func TestUsageLocalized(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	zh := Usage()
	if !strings.Contains(zh, "用法: dsh-desktop") || !strings.Contains(zh, "--stop-on-exit") {
		t.Fatalf("Usage() (zh) = %q", zh)
	}
	t.Setenv("LC_ALL", "en_US.UTF-8")
	en := Usage()
	if !strings.Contains(en, "Usage: dsh-desktop") || !strings.Contains(en, "--stop-on-exit") {
		t.Fatalf("Usage() (en) = %q", en)
	}
}

// TestEditDistance pins the transposition-aware distance the typo hint uses.
func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"port", "port", 0},
		{"prot", "port", 1},      // adjacent transposition
		{"updaetg", "update", 2}, // transposed t/e plus the stray g
		{"", "abc", 3},           // pure insertions
		{"startup-timeot", "startup-timeout", 1},
	}
	for _, tc := range cases {
		if got := editDistance(tc.a, tc.b); got != tc.want {
			t.Fatalf("editDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestWindowTitleText covers the default title template: the window title bar
// shows the running version, while an explicit --title stays verbatim unless it
// opts in with the {version} placeholder.
func TestWindowTitleText(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"default template", DefaultWindowTitle(), DefaultWindowTitleBase + " " + TitleVersion()},
		{"placeholder expanded", "My Harness {version}", "My Harness " + TitleVersion()},
		{"placeholder twice", "{version} / {version}", TitleVersion() + " / " + TitleVersion()},
		{"verbatim without placeholder", "My Harness", "My Harness"},
		{"verbatim with literal version", "My Harness 0.3.0", "My Harness 0.3.0"},
		{"surrounding whitespace trimmed", "  My Harness {version}  ", "My Harness " + TitleVersion()},
		{"blank falls back to default", "   ", DefaultWindowTitleBase + " " + TitleVersion()},
		{"empty falls back to default", "", DefaultWindowTitleBase + " " + TitleVersion()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.WindowTitle = tc.title
			if got := c.WindowTitleText(); got != tc.want {
				t.Fatalf("WindowTitleText() = %q, want %q", got, tc.want)
			}
		})
	}

	// A parsed run with no flags gets the versioned default title.
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := cfg.WindowTitleText(); got != DefaultWindowTitleBase+" "+TitleVersion() {
		t.Fatalf("default title = %q, want %q", got, DefaultWindowTitleBase+" "+TitleVersion())
	}
	if !strings.Contains(cfg.WindowTitleText(), Version) {
		t.Fatalf("default title %q must contain the running version %q", cfg.WindowTitleText(), Version)
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
