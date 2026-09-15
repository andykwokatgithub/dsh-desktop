// Package config holds the runtime options for the dsh-desktop shell.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Version is the application version. It can be overridden at build time:
//
//	go build -ldflags "-X github.com/deepseek-ai/dsh-desktop/internal/config.Version=0.3.2" .
var Version = "0.3.2"

// TitleVersion is the version token shown in the window title: the bare version
// ("0.3.2"), matching what --version prints.
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
// reads "DeepSeek Harness Desktop 0.3.2" and users can quote the version they run.
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
	fs := newFlagSet(&cfg)

	// The flag package would print its own English problem line and then the
	// usage, and main prints the localized error plus the usage itself -- that
	// would say everything twice. Silence the package and let Parse only return
	// the (localized) reason; the caller renders it exactly once.
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, err // main prints Usage() and exits 0
		}
		return nil, describeParseError(fs, err)
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

// newFlagSet registers the CLI flags on cfg and wires up the localized usage.
func newFlagSet(cfg *Config) *flag.FlagSet {
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
	return fs
}

// Usage renders the CLI help in the current UI language. It is what --help
// shows, and what a parameter error is followed by, so both are localized and
// printed exactly once by the caller.
func Usage() string {
	cfg := Default()
	var b strings.Builder
	fs := newFlagSet(&cfg)
	fs.SetOutput(&b)
	fs.Usage()
	return b.String()
}

// Localized parameter-error texts for the shapes the flag package produces. The
// package's own wording is always English, which would otherwise land in the
// middle of a Chinese line.
var (
	unknownFlagRe  = regexp.MustCompile(`flag provided but not defined: -+([^\s]+)`)
	invalidValueRe = regexp.MustCompile(`^invalid value ("[^"]*"|[^\s]+) for flag -+([^\s:]+): (.*)$`)
	invalidBoolRe  = regexp.MustCompile(`^invalid boolean value ("[^"]*"|[^\s]+) for -+([^\s:]+): (.*)$`)
	needsArgRe     = regexp.MustCompile(`^flag needs an argument: -+([^\s]+)$`)
	badSyntaxRe    = regexp.MustCompile(`^bad flag syntax: (.*)$`)
)

// describeParseError turns a flag-package parse error into the single localized
// line the user sees, keeping the useful detail (which flag, which value) and
// adding a "did you mean" hint for a mistyped flag. Unrecognized shapes fall
// back to the package's wording rather than losing information.
func describeParseError(fs *flag.FlagSet, err error) error {
	chinese := UseChineseUI()
	msg := err.Error()

	if m := unknownFlagRe.FindStringSubmatch(msg); m != nil {
		name := strings.ToLower(m[1])
		if chinese {
			text := fmt.Sprintf("未知参数 --%s", m[1])
			if hint, ok := suggestFlag(fs, name); ok {
				text += fmt.Sprintf("（是否想用 --%s？）", hint)
			}
			return errors.New(text)
		}
		text := fmt.Sprintf("unknown flag --%s", m[1])
		if hint, ok := suggestFlag(fs, name); ok {
			text += fmt.Sprintf(" (did you mean --%s?)", hint)
		}
		return errors.New(text)
	}
	if m := invalidValueRe.FindStringSubmatch(msg); m != nil {
		return localizedErr(chinese,
			fmt.Sprintf("参数 --%s 的取值 %s 无效", m[2], m[1]),
			fmt.Sprintf("invalid value %s for flag --%s", m[1], m[2]))
	}
	if m := invalidBoolRe.FindStringSubmatch(msg); m != nil {
		return localizedErr(chinese,
			fmt.Sprintf("参数 --%s 的布尔取值 %s 无效（应为 true/false）", m[2], m[1]),
			fmt.Sprintf("invalid boolean value %s for flag --%s (want true/false)", m[1], m[2]))
	}
	if m := needsArgRe.FindStringSubmatch(msg); m != nil {
		return localizedErr(chinese,
			fmt.Sprintf("参数 --%s 缺少取值", m[1]),
			fmt.Sprintf("flag --%s needs an argument", m[1]))
	}
	if m := badSyntaxRe.FindStringSubmatch(msg); m != nil {
		return localizedErr(chinese,
			fmt.Sprintf("参数写法有误: %s", m[1]),
			fmt.Sprintf("bad flag syntax: %s", m[1]))
	}
	return err
}

// suggestFlag returns the defined flag closest to the mistyped name, when it is
// close enough to be worth pointing at. A mistyped flag (e.g. "-updaetg",
// "--help11") is the single most common parameter mistake, and the flag package
// itself does not say which flag was probably meant.
func suggestFlag(fs *flag.FlagSet, typed string) (string, bool) {
	best, bestDist := "", 0
	consider := func(name string) {
		d := editDistance(typed, name)
		if best == "" || d < bestDist {
			best, bestDist = name, d
		}
	}
	// "help" is answered by the flag package itself (-h / -help / --help), so it
	// never shows up in VisitAll -- yet "--help11" is exactly the kind of typo
	// that should be pointed at it.
	consider("help")
	fs.VisitAll(func(f *flag.Flag) { consider(f.Name) })
	if best == "" || bestDist > suggestionDistance(len(typed)) {
		return "", false
	}
	return best, true
}

// localizedErr builds an error whose text matches the UI language.
func localizedErr(chinese bool, zh, en string) error {
	if chinese {
		return errors.New(zh)
	}
	return errors.New(en)
}

// suggestionDistance bounds how far the typed flag may be from a defined one
// before the hint becomes noise: one edit for short names, one more per five
// characters after that.
func suggestionDistance(n int) int {
	return 1 + n/5
}

// editDistance is the optimal-string-alignment distance: insertion, deletion,
// substitution and transposition of adjacent characters each cost 1, which is
// what typo detection needs -- "updaetg" -> "update" is 2 (transposed t/e plus
// the stray g), while plain Levenshtein would call it 3.
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev2 := make([]int, len(br)+1) // row i-2
	prev := make([]int, len(br)+1)  // row i-1
	cur := make([]int, len(br)+1)   // row i
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(br)]
}

// validate checks the semantic bounds of the parsed parameters so a
// misconfigured invocation fails loudly instead of misbehaving at runtime. An
// explicit --url is resolved first, because it may supply Host/Port. The
// messages follow the UI language, like the rest of the CLI output.
func (c *Config) validate() error {
	zh := UseChineseUI()
	if c.URL != "" {
		if err := c.applyURL(); err != nil {
			return err
		}
	}
	if c.Host == "" {
		return localizedErr(zh, "参数 --host 不能为空", "--host must not be empty")
	}
	if c.Host == "0.0.0.0" {
		return localizedErr(zh,
			"参数 --host 不支持 0.0.0.0（dsh 顶层已拒绝该绑定；请用 127.0.0.1）",
			"--host 0.0.0.0 is not supported (dsh itself refuses that bind; use 127.0.0.1)")
	}
	if c.Port < 1 || c.Port > 65535 {
		return localizedErr(zh,
			fmt.Sprintf("参数 --port 必须在 1-65535 之间, 当前为 %d", c.Port),
			fmt.Sprintf("--port must be between 1 and 65535, got %d", c.Port))
	}
	if c.WindowWidth <= 0 {
		return localizedErr(zh,
			fmt.Sprintf("参数 --width 必须大于 0, 当前为 %d", c.WindowWidth),
			fmt.Sprintf("--width must be greater than 0, got %d", c.WindowWidth))
	}
	if c.WindowHeight <= 0 {
		return localizedErr(zh,
			fmt.Sprintf("参数 --height 必须大于 0, 当前为 %d", c.WindowHeight),
			fmt.Sprintf("--height must be greater than 0, got %d", c.WindowHeight))
	}
	if c.StartupTimeoutSec <= 0 {
		return localizedErr(zh,
			fmt.Sprintf("参数 --startup-timeout 必须大于 0, 当前为 %d", c.StartupTimeoutSec),
			fmt.Sprintf("--startup-timeout must be greater than 0, got %d", c.StartupTimeoutSec))
	}
	if c.PollIntervalMS <= 0 {
		return localizedErr(zh,
			fmt.Sprintf("参数 --poll-ms 必须大于 0, 当前为 %d", c.PollIntervalMS),
			fmt.Sprintf("--poll-ms must be greater than 0, got %d", c.PollIntervalMS))
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
	zh := UseChineseUI()
	u, err := url.Parse(c.URL)
	if err != nil {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url 无效: %v", err),
			fmt.Sprintf("invalid --url: %v", err))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url 的 scheme 必须是 http/https, 当前为 %q", u.Scheme),
			fmt.Sprintf("--url scheme must be http or https, got %q", u.Scheme))
	}
	if u.Hostname() == "" {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url 缺少主机名: %q", c.URL),
			fmt.Sprintf("--url is missing a host name: %q", c.URL))
	}
	portText := u.Port()
	if portText == "" {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url 必须包含显式端口（例如 http://127.0.0.1:3080/?token=…）: %q", c.URL),
			fmt.Sprintf("--url must carry an explicit port (e.g. http://127.0.0.1:3080/?token=…): %q", c.URL))
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url 的端口无效: %q", portText),
			fmt.Sprintf("invalid port in --url: %q", portText))
	}
	if c.PortSet && port != c.Port {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url(%q) 的端口 %d 与 --port(%d) 不一致", c.URL, port, c.Port),
			fmt.Sprintf("--url (%q) port %d disagrees with --port (%d)", c.URL, port, c.Port))
	}
	if c.HostSet && !strings.EqualFold(u.Hostname(), c.Host) {
		return localizedErr(zh,
			fmt.Sprintf("参数 --url(%q) 的主机 %q 与 --host(%q) 不一致", c.URL, u.Hostname(), c.Host),
			fmt.Sprintf("--url (%q) host %q disagrees with --host (%q)", c.URL, u.Hostname(), c.Host))
	}
	c.Host = u.Hostname()
	c.Port = port
	return nil
}
