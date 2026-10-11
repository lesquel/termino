package cli

import (
	"fmt"

	"github.com/lesquel/termino/internal/platform/config"
	"github.com/spf13/cobra"
)

// knownThemes lists the themes bundled with termino.
var knownThemes = []string{
	"default",
	"spooky",
	"ocean",
	"forest",
	"tokyo",
}

// newThemeCmd returns `termino theme <name>`.
func newThemeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "theme <name>",
		Short: "Set the active colour theme",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if !isKnownTheme(name) {
				return fmt.Errorf("unknown theme %q — available themes: %v", name, knownThemes)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
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
	for _, t := range knownThemes {
		if t == name {
			return true
		}
	}
	return false
}
