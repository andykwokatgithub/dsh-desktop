package webview

import (
	"strings"
	"testing"
)

func TestSecurityInitCarriesTheAuthFenceDetector(t *testing.T) {
	js := SecurityInit(true)
	for _, want := range []string{AuthFenceText, AuthFenceBinding, OpenExternalBinding} {
		if !strings.Contains(js, want) {
			t.Fatalf("SecurityInit() output does not mention %q", want)
		}
	}
	// The context menu stays enabled by default, so the guard must be absent.
	if strings.Contains(js, "contextmenu") {
		t.Fatal("SecurityInit(true) should leave the context menu alone")
	}
	if js := SecurityInit(false); !strings.Contains(js, "contextmenu") {
		t.Fatal("SecurityInit(false) should disable the context menu")
	}
}

// TestBindingNamesAreStable guards the page/bridge contract: the embedded pages
// call these globals by name, so renaming one without the other breaks the UI
// silently.
func TestBindingNamesMatchThePages(t *testing.T) {
	if TokenSubmitBinding != "__dshSubmitToken" || TokenSkipBinding != "__dshSkipToken" {
		t.Fatal("token binding names changed; update internal/ui/token.html")
	}
	if TokenTargetBinding != "__dshTokenTarget" || TokenResultFunc != "__dshTokenResult" {
		t.Fatal("token target/result names changed; update internal/ui/token.html")
	}
	if ErrorFunc != "__dshError" || RetryBinding != "__dshRetry" {
		t.Fatal("error page names changed; update internal/ui/error.html")
	}
	if AuthFenceBinding != "__dshAuthRequired" {
		t.Fatal("auth fence binding name changed")
	}
}
