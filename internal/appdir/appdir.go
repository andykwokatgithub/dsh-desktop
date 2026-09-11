// Package appdir centralises the per-user state directory so the shell's log
// and its "which dsh web instance did I start" record live in one place.
package appdir

import (
	"os"
	"path/filepath"
)

// Name is the directory (and product name) used under %LOCALAPPDATA%.
const Name = "dsh-desktop"

// File names inside Dir().
const (
	LogFile      = "dsh-desktop.log"
	EndpointFile = "web-endpoint.json"
)

// Dir returns %LOCALAPPDATA%\dsh-desktop, creating it. When %LOCALAPPDATA% is
// unset (unusual, but possible in a service context) it falls back to the OS
// temp directory so the shell still has somewhere to log and to remember its
// endpoint.
func Dir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Path joins one of the file-name constants onto Dir(). It returns "" when the
// directory cannot be created, which callers treat as "feature unavailable"
// rather than as a fatal error.
func Path(name string) string {
	dir, err := Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, name)
}
