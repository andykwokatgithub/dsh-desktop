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
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultRepo is the GitHub "owner/repo" that owns the release assets.
const DefaultRepo = "andykwokatgithub/dsh-desktop"

// Asset names published by the release pipeline.
const (
	AssetExe   = "dsh-desktop-win-x64.exe"
	AssetSHA   = "dsh-desktop-win-x64.exe.sha256"
	apiBaseURL = "https://api.github.com"
)

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

// Latest resolves the most recent stable GitHub release for repo.
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
// is never left behind.
func Download(ctx context.Context, assetURL, expectedSHA, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download failed: HTTP %d", resp.StatusCode)
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
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA, actual)
	}
	return os.Rename(staged, dest)
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

// FetchSHA downloads the SHA sidecar asset for a release and returns its digest.
func FetchSHA(ctx context.Context, shaURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shaURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
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
