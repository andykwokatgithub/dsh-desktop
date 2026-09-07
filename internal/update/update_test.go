package update

import (
	"testing"
)

func TestSemverCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.7", "0.2.8", -1},
		{"0.2.8", "0.2.7", 1},
		{"0.2.8", "0.2.8", 0},
		{"v0.2.8", "0.2.8", 0},     // leading "v" tolerated
		{"1.0.0", "0.9.9", 1},      // major
		{"0.3.0", "0.2.9", 1},      // minor beats patch
		{"0.2.10", "0.2.9", 1},     // numeric, not lexicographic
		{"0.2.8-beta", "0.2.8", 0}, // pre-release suffix ignored
	}
	for _, tc := range cases {
		if got := SemverCompare(tc.a, tc.b); got != tc.want {
			t.Errorf("SemverCompare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestNewerThan(t *testing.T) {
	if !NewerThan("0.2.7", "0.2.8") {
		t.Error("NewerThan(0.2.7, 0.2.8) should be true")
	}
	if NewerThan("0.2.8", "0.2.8") {
		t.Error("NewerThan(0.2.8, 0.2.8) should be false")
	}
	if NewerThan("0.2.9", "0.2.8") {
		t.Error("NewerThan(0.2.9, 0.2.8) should be false")
	}
}

func TestLoadSHA(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"ABC123  dsh-desktop-win-x64.exe\n", "abc123"},
		{"  DEF456  dsh-desktop-win-x64.exe  \n", "def456"},
		{"# comment\n\n 1A2B dsh-desktop-win-x64.exe\n", "1a2b"},
		{"", ""},
		{"\n# only comments\n\n", ""},
	}
	for _, tc := range cases {
		if got := LoadSHA(tc.body); got != tc.want {
			t.Errorf("LoadSHA(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestReleaseVersion(t *testing.T) {
	r := &Release{TagName: "v0.2.8"}
	if got := r.Version(); got != "0.2.8" {
		t.Errorf("Release.Version() = %q, want %q", got, "0.2.8")
	}
}

func TestReleaseAssetURL(t *testing.T) {
	r := &Release{Assets: []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{{Name: "dsh-desktop-win-x64.exe", BrowserDownloadURL: "https://x/y.exe"}}}
	if got := r.AssetURL("dsh-desktop-win-x64.exe"); got != "https://x/y.exe" {
		t.Errorf("AssetURL() = %q, want the URL", got)
	}
	if got := r.AssetURL("missing"); got != "" {
		t.Errorf("AssetURL(missing) = %q, want empty", got)
	}
}
