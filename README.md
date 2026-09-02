# dsh-desktop

A Go + WebView2 desktop shell for the DeepSeek Harness Web UI.

It embeds `dsh web` (served at `http://127.0.0.1:3080`) in a native window,
manages the backend service lifecycle, and enforces a single running instance.

Docs:
- Requirement baseline: [`docs/dsh-destop-prd.md`](docs/dsh-destop-prd.md)
- Technical design: [`docs/dsh-desktop-technical-design.md`](docs/dsh-desktop-technical-design.md)

## Requirements

- Windows 10 (1803+) / Windows 11
- WebView2 Evergreen runtime
- `dsh` installed globally (e.g. `npm i -g @deepseek-ai/dsh`) with `node` and
  `dsh` on `PATH`.

## Build

Recommended (also embeds the DeepSeek Harness icon):

```powershell
.\build.ps1
```

Or directly:

```powershell
go build -ldflags "-H=windowsgui" -o dsh-desktop.exe .
```

The `-H=windowsgui` linker flag makes the binary a **Windows GUI-subsystem**
app so no black console (SHELL) window appears when it is launched by
double-click. Without it the binary is a console app and Windows allocates a
console window.

### Application icon

The executable embeds the DeepSeek Harness whale mark on a DeepSeek-blue
rounded square. Assets and tooling:

- `assets/deepseek.ico` — the multi-resolution icon (16..256px).
- `assets/deepseek-256.png` — a 256px PNG preview.
- `tools/make-icon.mjs` — regenerates `deepseek.ico` from the official
  `dsh-web-frontend/dist/favicon.svg` using `sharp`.
- `rsrc_windows_amd64.syso` — resource object embedding the icon; consumed
  automatically by `go build` for `windows/amd64`. `build.ps1` regenerates it
  (via `github.com/akavel/rsrc`) when the `.ico` is newer.

Notes:
- `github.com/webview/webview_go` uses CGO, so a Windows toolchain with GCC is
  required; cross-compiling from another OS is not supported.
- The built binary needs the WebView2 runtime at run time.
- The spawned `dsh web` backend is itself launched hidden (`CREATE_NO_WINDOW`),
  so it does not flash a console either.

## Run

```powershell
.\dsh-desktop.exe
```

Flags (mirror the config surface): `-url`, `-host`, `-port`, `-command`,
`-stop-on-exit`, `-devtools`, `-context-menu`, `-startup-timeout`, `-poll-ms`,
`-width`, `-height`, `-title`.

## Layout

```
main.go                          entry point (orchestration)
internal/config/                 runtime flags & defaults
internal/service/                dsh probe / spawn / lifecycle
internal/singleinstance/         mutex + native window + activation message
internal/webview/                security scripts & external-link handoff
internal/ui/                     embedded loading / error pages (go:embed)
```
