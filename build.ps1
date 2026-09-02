# Build dsh-desktop.exe as a Windows GUI-subsystem app (no console window),
# embedding the DeepSeek Harness icon.
# Requires Go with CGO + a GCC toolchain (e.g. mingw-w64) on PATH, and (only to
# regenerate the icon resources) Node with sharp.
param(
    # Optional version to embed via -ldflags (-X internal/config.Version).
    # When set, the main package builds with this version reported by -version.
    [string]$Version = ""
)
$ErrorActionPreference = "Stop"

# Use workspace-local module/build cache so the repo builds in sandboxed envs.
$env:GOPATH = Join-Path $PSScriptRoot ".gopath"
$env:GOMODCACHE = Join-Path $env:GOPATH "pkg\mod"
$env:GOCACHE = Join-Path $env:GOPATH "buildcache"

$icon  = Join-Path $PSScriptRoot "assets\deepseek.ico"
$syso  = Join-Path $PSScriptRoot "rsrc_windows_amd64.syso"
$out   = Join-Path $PSScriptRoot "dsh-desktop.exe"

if (-not (Test-Path $icon)) {
    Write-Error "Missing $icon. Regenerate it with:  node tools/make-icon.mjs"
    exit 1
}

# Regenerate the resource object only when the icon is newer (idempotent).
$needSyso = -not (Test-Path $syso) -or ((Get-Item $icon).LastWriteTime -gt (Get-Item $syso).LastWriteTime)
if ($needSyso) {
    Write-Host "==> generating resource object (rsrc)"
    go run github.com/akavel/rsrc@latest -ico $icon -arch amd64 -o $syso
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

Write-Host "==> go build (GUI subsystem + icon)"
$ldflags = "-H=windowsgui"
if ($Version -ne "") {
    $ldflags += " -X github.com/deepseek-ai/dsh-desktop/internal/config.Version=$Version"
}
go build -ldflags $ldflags -o $out .
if ($LASTEXITCODE -eq 0) {
    Write-Host "Built $out"
}
exit $LASTEXITCODE
