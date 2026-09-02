// Package webview adds the DSH-specific behaviours on top of the low-level
// WebView2 binding: page-load security scripts and the external-link handoff.
//
// Security model (PRD V1.1 / D7): only the canonical local origin
// (http://127.0.0.1:<port>) is trusted. External http(s) and mailto: links are
// handed to the system default browser. The high-level webview binding does not
// expose navigation events or ICoreWebView2Settings, so these are implemented
// at the DOM/JS level here; the full control surface requires a lower-level
// binding (see PRD §4.6).
package webview

import (
	"os/exec"

	webviewlib "github.com/webview/webview_go"
)

// SecurityInit returns the JavaScript injected into every page. It disables the
// WebView2 DOM context menu (when enabled is false) and routes external links
// to the bound `__dshOpenExternal` bridge.
func SecurityInit(enableContextMenu bool) string {
	js := ""
	if !enableContextMenu {
		js += `document.addEventListener('contextmenu', function(e){ e.preventDefault(); });` + "\n"
	}
	js += `
document.addEventListener('click', function(e){
  var a = e.target && e.target.closest && e.target.closest('a');
  if(!a) return;
  var href = a.href || '';
  var isExternal = /^(https?|mailto):/i.test(href) && (href.indexOf(location.origin) !== 0);
  if(isExternal){
    e.preventDefault();
    if(window.__dshOpenExternal) window.__dshOpenExternal(href);
  }
});
`
	return js
}

// BindExternal registers the bridge that opens a URL in the default browser.
func BindExternal(view webviewlib.WebView) error {
	open := func(url string) (string, error) {
		// `start "" <url>` opens the URL with the system handler.
		cmd := exec.Command("cmd", "/c", "start", "", url)
		_ = cmd.Start()
		return "", nil
	}
	return view.Bind("__dshOpenExternal", open)
}
