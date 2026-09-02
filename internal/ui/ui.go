// Package ui embeds the local loading/error surfaces shown before the
// dsh web UI is ready.
package ui

import _ "embed"

//go:embed loading.html
var Loading string

//go:embed error.html
var Error string
