package tui

import "github.com/lesquel/termino/internal/tuikit"

// KeyMap is a type alias for tuikit.KeyMap.
type KeyMap = tuikit.KeyMap

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return tuikit.DefaultKeyMap()
}
