// Package update implements a minimal, verifiable self-updater for the
// dsh-desktop Windows binary. It resolves the latest GitHub release for the
// repository, compares it against the embedded version, and can download +
// verify + stage a new binary. It is intentionally small and dependency-free
// (standard library + x/sys/windows) so the GUI binary stays lean.
//
// Distribution/update model:
//   - The release pipeline (see .github/workflows/release.yml) attaches
//     `dsh-desktop-win-x64.exe` and `dsh-desktop-win-x64.exe.sha256` assets.
//   - The npm package and `dsh plugin` use @andykwok/dsh-desktop to (re)install
//     the binary; this package is the update path for users who grabbed the EXE
//     directly from GitHub Releases.
//
// Networking robustness: GitHub release *assets* are served from a CDN that is
// often slow or blocked in some regions even though the api.github.com metadata
// endpoint stays reachable. To keep the updater usable there, downloads try the
// official URL, then a detected local proxy (env vars or a reachable common
// Clash/V2Ray port such as 127.0.0.1:7890), then domestic GitHub mirrors. Every
// attempt has its own bounded timeout so one hanging host cannot eat the whole
// update window, and integrity is still enforced by the SHA256 sidecar fetched
// from the official release metadata (so a mirror can never serve an unverified
// binary).
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultRepo is the GitHub "owner/repo" that owns the release assets.
const DefaultRepo = "andykwokatgithub/dsh-desktop"

// Asset names published by the release pipeline.
const (
	AssetExe   = "dsh-desktop-win-x64.exe"
	AssetSHA   = "dsh-desktop-win-x64.exe.sha256"
	apiBaseURL = "https://api.github.com"
)

var (
	errHashMismatch   = errors.New("sha256 mismatch")
	attemptTimeout    = 45 * time.Second // per download attempt
	retryAttempts     = 1                // tries per candidate URL (many candidates)
	proxyProbeTimeout = 350 * time.Millisecond
)

// mirrorPrefixes are prepended to a GitHub release asset URL to route the
// download through a domestic mirror/proxy when the official CDN is slow or
// blocked. Order = preference; the first candidate that succeeds wins, so the
// known-most-reliable mirrors are listed first. They only affect the binary
// download, whose integrity is still guaranteed by the SHA256 sidecar fetched
// independently from the official release metadata.
var mirrorPrefixes = []string{
	"https://ghfast.top/",
	"https://ghproxy.net/",
	"https://ghproxy.com/",
	"https://gh-proxy.com/",
}

// Release is the subset of a GitHub release this package cares about.
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// downloadAttempt is a single (URL, client) pair to try for an asset fetch.
// We try the official URL directly, optionally through a detected local proxy,
// then each domestic mirror.
type downloadAttempt struct {
	url    string
	client *http.Client
}

// Version returns the semver version (no leading "v") for a release tag.
func (r *Release) Version() string {
	return strings.TrimPrefix(r.TagName, "v")
}

// AssetURL returns the browser_download_url for a named asset, if present.
func (r *Release) AssetURL(name string) string {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// Latest resolves the most recent stable GitHub release for repo. It uses the
// default client (env proxies only) because api.github.com is normally reachable
// directly and must not be forced through an auto-detected local proxy.
func Latest(ctx context.Context, repo string) (*Release, error) {
	if repo == "" {
		repo = DefaultRepo
	}
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBaseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release lookup failed: HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// SemverCompare returns -1, 0, or 1 when a is older than, equal to, or newer
// than b. It compares numeric dotted triples and tolerates a leading "v".
func SemverCompare(a, b string) int {
	pa := parseVersion(a)
	pb := parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseVersion(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(parts[i], "-", 2)[0]))
		if err != nil {
			n = 0
		}
		out[i] = n
	}
	return out
}

// NewerThan reports whether latest is a strictly greater version than current.
func NewerThan(current, latest string) bool {
	return SemverCompare(latest, current) > 0
}

// Download fetches an asset to dest (streamed), verifies it against the
// expected hex SHA256, and only then renames it into place. A hash mismatch
// removes the staged file and returns an error, so a corrupt/tampered download
// is never left behind. It iterates the candidate list (official → local proxy →
// mirrors), giving each attempt its own timeout and failing over on error.
func Download(ctx context.Context, assetURL, expectedSHA, dest string) error {
	var lastErr error
	for _, a := range downloadAttempts(assetURL) {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := tryDownload(ctx, a.client, a.url, expectedSHA, dest)
		if err == nil {
			return nil
		}
		if errors.Is(err, errHashMismatch) {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// FetchSHA downloads the SHA sidecar asset for a release and returns its digest.
// It uses the same failover (official → local proxy → mirrors) so the hash is
// still obtainable when the official CDN is blocked.
func FetchSHA(ctx context.Context, shaURL string) (string, error) {
	var lastErr error
	for _, a := range downloadAttempts(shaURL) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		s, err := fetchSHAOnce(ctx, a.client, a.url)
		if err == nil {
			return s, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func fetchSHAOnce(ctx context.Context, cli *http.Client, shaURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shaURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SHA asset download failed: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return LoadSHA(string(b)), nil
}

// LoadSHA reads a "sha  filename" line from a release SHA asset body and
// returns the hex digest (lowercased).
func LoadSHA(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

// ErrNoNewer is returned when the current version is already up to date.
var ErrNoNewer = errors.New("already up to date")

// Resolve returns the download URL + expected hash for the latest release,
// validating that a newer version than current exists.
func Resolve(ctx context.Context, repo, current string) (exeURL, sha, latest string, err error) {
	rel, err := Latest(ctx, repo)
	if err != nil {
		return "", "", "", err
	}
	latest = rel.Version()
	if !NewerThan(current, latest) {
		return "", "", latest, ErrNoNewer
	}
	exeURL = rel.AssetURL(AssetExe)
	if exeURL == "" {
		return "", "", latest, fmt.Errorf("release %s has no %s asset", rel.TagName, AssetExe)
	}
	if shaURL := rel.AssetURL(AssetSHA); shaURL != "" {
		if s, e := FetchSHA(ctx, shaURL); e == nil {
			sha = s
		}
	}
	return exeURL, sha, latest, nil
}

// downloadAttempts returns the fetch candidates in order: the official URL
// through a detected local proxy (if any), then the official URL directly, then
// each domestic mirror. Preferring the local proxy honors an existing Clash/V2Ray
// setup without hard-depending on it; if it is not actually working the attempt
// fails fast and we fall through.
func downloadAttempts(assetURL string) []downloadAttempt {
	out := make([]downloadAttempt, 0, 2+len(mirrorPrefixes))
	if p := localProxyClient(); p != nil {
		out = append(out, downloadAttempt{url: assetURL, client: p})
	}
	out = append(out, downloadAttempt{url: assetURL, client: http.DefaultClient})
	for _, m := range mirrorPrefixes {
		out = append(out, downloadAttempt{url: m + assetURL, client: http.DefaultClient})
	}
	return out
}

// tryDownload attempts a single candidate URL up to retryAttempts times, each
// with its own attemptTimeout so one hanging request cannot eat the whole window.
func tryDownload(parent context.Context, cli *http.Client, assetURL, expectedSHA, dest string) error {
	var lastErr error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if err := parent.Err(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(parent, attemptTimeout)
		err := downloadOnce(ctx, cli, assetURL, expectedSHA, dest)
		cancel()
		if err == nil {
			return nil
		}
		if errors.Is(err, errHashMismatch) {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// downloadOnce is a single, un-retried fetch + verify of one asset URL.
func downloadOnce(ctx context.Context, cli *http.Client, assetURL, expectedSHA, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download failed: HTTP %d (%s)", resp.StatusCode, assetURL)
	}

	if dir := filepath.Dir(dest); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	staged := dest + ".download"
	f, err := os.Create(staged)
	if err != nil {
		return err
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		_ = os.Remove(staged)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(staged)
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if expectedSHA != "" && !strings.EqualFold(actual, expectedSHA) {
		_ = os.Remove(staged)
		return fmt.Errorf("%w: expected %s, got %s", errHashMismatch, expectedSHA, actual)
	}
	return os.Rename(staged, dest)
}

// localProxyClient returns an *http.Client that routes through a discovered local
// proxy, or nil when none is found. The proxy is only used as one candidate among
// several, so a misconfigured/stalled proxy does not block the update.
func localProxyClient() *http.Client {
	if p := localProxy(); p != "" {
		if pu, err := url.Parse(p); err == nil {
			return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
		}
	}
	return nil
}

// localProxy returns a proxy URL string when a relevant proxy env var is set, or
// when one of the common local proxy ports (Clash/V2Ray defaults) is reachable on
// 127.0.0.1. It returns "" otherwise.
func localProxy() string {
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	for _, port := range []string{"7890", "7891", "10809", "1080", "8080", "8888"} {
		if tcpReachable("127.0.0.1", port, proxyProbeTimeout) {
			return "http://127.0.0.1:" + port
		}
	}
	return ""
}

// tcpReachable reports whether a TCP connection to host:port can be opened
// within timeout. Used to detect a local proxy without waiting long.
func tcpReachable(host, port string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
