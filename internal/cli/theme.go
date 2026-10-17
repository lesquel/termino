package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/tuikit"
	"github.com/spf13/cobra"
)

// userThemesDir is where user themes ($XDG_CONFIG_HOME/termino/themes) live.
func userThemesDir() string {
	return filepath.Join(config.ConfigDir(), "themes")
}

// availableThemes lists the embedded themes plus the user's own.
func availableThemes() []string {
	return tuikit.Available(userThemesDir())
}

// newThemeCmd returns `termino theme [name]`: without a name it lists the
// available themes and marks the current one.
func newThemeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "theme [name]",
		Short: "List colour themes, or set the active one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			current := cfg.Theme
			if current == "" {
				current = "default"
			}

			if len(args) == 0 {
				for _, name := range availableThemes() {
					mark := " "
					if name == current {
						mark = "*"
					}
					fmt.Printf("%s %s\n", mark, name)
				}
				return nil
			}

			name := strings.TrimSpace(args[0])
			if !isKnownTheme(name) {
				return fmt.Errorf("unknown theme %q — available themes: %s", name, strings.Join(availableThemes(), ", "))
			}
			cfg.Theme = name
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Printf("Theme set to %q\n", name)
			return nil
		},
	}
}

func isKnownTheme(name string) bool {
	for _, t := range availableThemes() {
		if t == name {
			return true
		}
	}
	return false
}
