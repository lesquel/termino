// Package tui contains the root Bubble Tea model and shared TUI utilities.
package tui

import (
	"path/filepath"

	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/tuikit"
)

// Theme is a type alias for tuikit.Theme so callers in this package don't
// need to import tuikit directly.
type Theme = tuikit.Theme

// Load loads a theme by name, checking the user themes dir first then embedded.
func Load(name string) (*Theme, error) {
	userThemesDir := filepath.Join(config.ConfigDir(), "themes")
	return tuikit.Load(name, userThemesDir)
}
