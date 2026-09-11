package ui

import (
	"strings"
	"testing"
)

// TestEmbeddedPagesKeepTheirBridgeContract pins the JavaScript globals the
// shell binds to: a page and its Go bridge must change together.
func TestEmbeddedPagesKeepTheirBridgeContract(t *testing.T) {
	cases := []struct {
		name  string
		page  string
		needs []string
	}{
		{
			name: "token page",
			page: Token,
			needs: []string{
				"__dshSubmitToken", "__dshSkipToken", "__dshTokenTarget", "__dshTokenResult",
			},
		},
		{
			name:  "error page",
			page:  Error,
			needs: []string{"__dshError", "__dshRetry"},
		},
		{
			name:  "loading page",
			page:  Loading,
			needs: []string{"DeepSeek Harness"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.TrimSpace(tc.page) == "" {
				t.Fatal("embedded page is empty")
			}
			for _, want := range tc.needs {
				if !strings.Contains(tc.page, want) {
					t.Fatalf("page does not reference %q", want)
				}
			}
		})
	}
}

// TestEmbeddedPagesLoadNoRemoteContent keeps the local pages consistent with the
// navigation whitelist: they may link out (the click interceptor hands external
// links to the browser) but they must never *load* a remote resource.
func TestEmbeddedPagesLoadNoRemoteContent(t *testing.T) {
	for name, page := range map[string]string{"loading": Loading, "error": Error, "token": Token} {
		lower := strings.ToLower(page)
		for _, banned := range []string{`src="http`, `src='http`, `@import`, `url(http`} {
			if strings.Contains(lower, banned) {
				t.Fatalf("%s page loads remote content (%q)", name, banned)
			}
		}
	}
}
