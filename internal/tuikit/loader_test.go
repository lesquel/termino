package tuikit

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// builtinThemes are the themes termino ships; each must stay embedded.
var builtinThemes = []string{
	"default", "spooky", "dracula", "nord", "gruvbox",
	"catppuccin-mocha", "tokyo-night", "solarized-light", "high-contrast",
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// TestBuiltinThemesAreValid guards the embedded themes (the single source of
// truth for built-in themes): every file must decode strictly, define every
// color as a #RRGGBB hex value, use a known border style, and load via Load.
func TestBuiltinThemesAreValid(t *testing.T) {
	names := Available("")
	for _, want := range builtinThemes {
		if !slices.Contains(names, want) {
			t.Fatalf("built-in theme %q missing; got %v", want, names)
		}
	}

	knownBorders := map[string]bool{"": true, "rounded": true, "normal": true, "thick": true, "double": true, "hidden": true}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			data, err := embeddedThemes.ReadFile("themes/" + name + ".toml")
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			var tf themeFile
			dec := toml.NewDecoder(bytes.NewReader(data))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&tf); err != nil {
				t.Fatalf("strict decode: %v", err)
			}

			c := tf.Colors
			colors := map[string]string{
				"surface": c.Surface, "surface_alt": c.SurfaceAlt, "text": c.Text,
				"text_muted": c.TextMuted, "accent": c.Accent, "success": c.Success,
				"warning": c.Warning, "danger": c.Danger, "border": c.Border, "done": c.Done,
			}
			for key, v := range colors {
				if !hexColor.MatchString(v) {
					t.Errorf("colors.%s = %q, want #RRGGBB", key, v)
				}
			}

			for _, sym := range []string{"done", "high", "critical", "medium", "low"} {
				if strings.TrimSpace(tf.Symbols[sym]) == "" {
					t.Errorf("symbols.%s missing", sym)
				}
			}

			if !knownBorders[tf.Options.BorderStyle] {
				t.Errorf("options.border_style = %q is not a known style", tf.Options.BorderStyle)
			}

			if _, err := Load(name, ""); err != nil {
				t.Errorf("Load: %v", err)
			}
		})
	}
}
