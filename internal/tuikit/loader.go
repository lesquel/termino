package tuikit

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/pelletier/go-toml/v2"
)

//go:embed themes/*.toml
var embeddedThemes embed.FS

// themeFile is the structure of a .toml theme file.
type themeFile struct {
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
	} `toml:"options"`
}

// Load loads a theme by name. It tries userThemesDir first, then embedded themes.
// Pass an empty userThemesDir to skip user themes.
func Load(name, userThemesDir string) (*Theme, error) {
	if name == "" {
		name = "default"
	}
	filename := name + ".toml"

	// Try user themes directory first.
	if userThemesDir != "" {
		userPath := filepath.Join(userThemesDir, filename)
		if data, err := os.ReadFile(userPath); err == nil {
			return parseTheme(data)
		}
	}

	// Fall back to embedded themes.
	embeddedPath := "themes/" + filename
	data, err := embeddedThemes.ReadFile(embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("theme %q not found: %w", name, err)
	}
	return parseTheme(data)
}

func parseTheme(data []byte) (*Theme, error) {
	var tf themeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parse theme: %w", err)
	}

	border := lipgloss.RoundedBorder()
	switch tf.Options.BorderStyle {
	case "normal":
		border = lipgloss.NormalBorder()
	case "thick":
		border = lipgloss.ThickBorder()
	case "double":
		border = lipgloss.DoubleBorder()
	case "hidden":
		border = lipgloss.HiddenBorder()
	}

	symbols := tf.Symbols
	if symbols == nil {
		symbols = map[string]string{
			"done":     "✓",
			"high":     "▲",
			"critical": "!!!",
			"medium":   "–",
			"low":      "▼",
		}
	}

	return &Theme{
		Surface:     lipgloss.Color(tf.Colors.Surface),
		SurfaceAlt:  lipgloss.Color(tf.Colors.SurfaceAlt),
		Text:        lipgloss.Color(tf.Colors.Text),
		TextMuted:   lipgloss.Color(tf.Colors.TextMuted),
		Accent:      lipgloss.Color(tf.Colors.Accent),
		Success:     lipgloss.Color(tf.Colors.Success),
		Warning:     lipgloss.Color(tf.Colors.Warning),
		Danger:      lipgloss.Color(tf.Colors.Danger),
		Border:      lipgloss.Color(tf.Colors.Border),
		Done:        lipgloss.Color(tf.Colors.Done),
		BorderStyle: border,
		Symbols:     symbols,
	}, nil
}
