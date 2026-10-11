// Package themes exposes the embedded TOML theme files and provides a
// LoadEmbedded helper. The canonical theme loader used by the TUI is in
// internal/tuikit; this package makes the root-level theme files accessible
// for documentation and tooling purposes.
package themes

import (
	"embed"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/pelletier/go-toml/v2"
)

//go:embed *.toml
var FS embed.FS

// Theme holds the parsed color, symbol, and option values from a theme file.
type Theme struct {
	Name    string
	Colors  ThemeColors
	Symbols map[string]string
	Options ThemeOptions
}

// ThemeColors holds the semantic color tokens.
type ThemeColors struct {
	Surface    lipgloss.Color
	SurfaceAlt lipgloss.Color
	Text       lipgloss.Color
	TextMuted  lipgloss.Color
	Accent     lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Danger     lipgloss.Color
	Border     lipgloss.Color
	Done       lipgloss.Color
}

// ThemeOptions holds display behaviour options.
type ThemeOptions struct {
	BorderStyle string
	Motion      bool
}

// raw is the TOML file structure.
type raw struct {
	Colors struct {
		Surface    string `toml:"surface"`
		SurfaceAlt string `toml:"surface_alt"`
		Text       string `toml:"text"`
		TextMuted  string `toml:"text_muted"`
		Accent     string `toml:"accent"`
		Success    string `toml:"success"`
		Warning    string `toml:"warning"`
		Danger     string `toml:"danger"`
		Border     string `toml:"border"`
		Done       string `toml:"done"`
	} `toml:"colors"`
	Symbols map[string]string `toml:"symbols"`
	Options struct {
		BorderStyle string `toml:"border_style"`
		Motion      bool   `toml:"motion"`
	} `toml:"options"`
}

// LoadEmbedded loads a theme by name from the embedded FS.
// name should be "default" or "spooky" (without the .toml extension).
func LoadEmbedded(name string) (*Theme, error) {
	if name == "" {
		name = "default"
	}
	data, err := FS.ReadFile(name + ".toml")
	if err != nil {
		return nil, fmt.Errorf("theme %q not found in embedded FS: %w", name, err)
	}
	return parse(name, data)
}

func parse(name string, data []byte) (*Theme, error) {
	var r raw
	if err := toml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse theme %q: %w", name, err)
	}
	syms := r.Symbols
	if syms == nil {
		syms = map[string]string{
			"done":     "✓",
			"high":     "▲",
			"critical": "!!!",
			"medium":   "–",
			"low":      "▼",
		}
	}
	return &Theme{
		Name: name,
		Colors: ThemeColors{
			Surface:    lipgloss.Color(r.Colors.Surface),
			SurfaceAlt: lipgloss.Color(r.Colors.SurfaceAlt),
			Text:       lipgloss.Color(r.Colors.Text),
			TextMuted:  lipgloss.Color(r.Colors.TextMuted),
			Accent:     lipgloss.Color(r.Colors.Accent),
			Success:    lipgloss.Color(r.Colors.Success),
			Warning:    lipgloss.Color(r.Colors.Warning),
			Danger:     lipgloss.Color(r.Colors.Danger),
			Border:     lipgloss.Color(r.Colors.Border),
			Done:       lipgloss.Color(r.Colors.Done),
		},
		Symbols: syms,
		Options: ThemeOptions{
			BorderStyle: r.Options.BorderStyle,
			Motion:      r.Options.Motion,
		},
	}, nil
}
