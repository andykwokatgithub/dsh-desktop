// Package webview adds the DSH-specific behaviours on top of the low-level
// WebView2 binding: page-load security scripts, the external-link handoff, and
// the bridges the local HTML pages (token input, error/retry) call back through.
//
// Security model (PRD V1.2 / D7): only the canonical local origin
// (http://<host>:<port>) is trusted. External http(s) and mailto: links are
// handed to the system default browser. The high-level webview binding does not
// expose navigation events or ICoreWebView2Settings, so these are implemented
// at the DOM/JS level here; the full control surface requires a lower-level
// binding (see PRD §4.6).
package webview

import (
	"os/exec"
	"syscall"

	webviewlib "github.com/webview/webview_go"
)

// JavaScript globals the shell and the embedded pages agree on. The page -> Go
// direction is a bound function (a promise in the page); the Go -> page
// direction is a plain function the page defines and the shell calls with Eval.
const (
	OpenExternalBinding = "__dshOpenExternal"
	AuthFenceBinding    = "__dshAuthRequired"
	TokenTargetBinding  = "__dshTokenTarget"
	TokenSubmitBinding  = "__dshSubmitToken"
	TokenSkipBinding    = "__dshSkipToken"
	RetryBinding        = "__dshRetry"

	// TokenResultFunc and ErrorFunc are page-defined callbacks.
	TokenResultFunc = "__dshTokenResult"
	ErrorFunc       = "__dshError"
)

// AuthFenceText is the fixed body dsh's browser-auth boundary returns for an
// unauthenticated index request. Detecting it is how the shell learns that the
// browser cookie no longer authorises the page.
const AuthFenceText = "dsh web authentication required"

// TokenTarget is the endpoint the token page is asking about.
type TokenTarget struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// TokenResult is the acknowledgement a bound page call returns. The token page
// uses Message only for immediate feedback; the real verdict arrives later
// through TokenResultFunc.
type TokenResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// SecurityInit returns the JavaScript injected into every page: the
// context-menu switch, the external-link handoff, and the browser-auth fence
// detector.
//
// The fence detector is what keeps an expired/absent browser cookie away from
// dsh's plain-text 401 page: it reports the page URL once per document, and the
// shell decides whether to re-mint the cookie (own instance) or ask the user for
// a token (somebody else's instance).
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
    if(window.` + OpenExternalBinding + `) window.` + OpenExternalBinding + `(href);
  }
});
`
	js += `
(function(){
  if(window.__dshAuthFenceGuard) return;
  window.__dshAuthFenceGuard = true;
  var reported = false;
  function check(){
    if(reported) return;
    var text = (document.body && document.body.textContent) || '';
    if(text.indexOf('` + AuthFenceText + `') === -1) return;
    reported = true;
    if(window.` + AuthFenceBinding + `) window.` + AuthFenceBinding + `(location.href);
  }
  if(document.readyState === 'loading'){
    document.addEventListener('DOMContentLoaded', check);
  } else {
    check();
  }
  window.addEventListener('load', check);
})();
`
	return js
}

// BindExternal registers the bridge that opens a URL in the default browser.
func BindExternal(view webviewlib.WebView) error {
	open := func(url string) (string, error) {
		// `start "" <url>` opens the URL with the system default handler.
		cmd := exec.Command("cmd", "/c", "start", "", url)
		// The host process is a GUI subsystem binary (no console). cmd.exe is a
		// console-subsystem program, so without CREATE_NO_WINDOW a new console
		// window flashes for every external link opened. Mirror the service
		// spawn: suppress it so only the default browser appears.
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
		_ = cmd.Start()
		return "", nil
	}
	return view.Bind(OpenExternalBinding, open)
}

// BindAuthGuard registers the browser-auth fence bridge. The handler must not
// block: webview_go invokes bound functions on the UI thread, so the shell's
// callback is started on its own goroutine and the page call returns at once.
func BindAuthGuard(view webviewlib.WebView, onFence func(pageURL string)) error {
	return view.Bind(AuthFenceBinding, func(pageURL string) (string, error) {
		go onFence(pageURL)
		return "", nil
	})
}

// BindTokenTarget registers the "which endpoint is this page asking about" call.
// It is cheap and answered immediately.
func BindTokenTarget(view webviewlib.WebView, target func() TokenTarget) error {
	return view.Bind(TokenTargetBinding, func() (TokenTarget, error) {
		return target(), nil
	})
}

// BindTokenSubmit registers the token page's submit bridge. The callback must
// return immediately and continue on a goroutine: the page shows "validating…"
// meanwhile and receives the verdict through TokenResultFunc.
func BindTokenSubmit(view webviewlib.WebView, submit func(pasted string) TokenResult) error {
	return view.Bind(TokenSubmitBinding, func(pasted string) (TokenResult, error) {
		return submit(pasted), nil
	})
}

// BindTokenSkip registers the token page's "use my own instance" bridge.
func BindTokenSkip(view webviewlib.WebView, skip func() TokenResult) error {
	return view.Bind(TokenSkipBinding, func() (TokenResult, error) {
		return skip(), nil
	})
}

// BindRetry registers the error page's retry bridge.
func BindRetry(view webviewlib.WebView, retry func() TokenResult) error {
	return view.Bind(RetryBinding, func() (TokenResult, error) {
		return retry(), nil
	})
}
