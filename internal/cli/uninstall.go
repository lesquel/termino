package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newUninstallCmd returns `termino uninstall` — stub for FEAT-005.
func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Termino data and integrations — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Uninstall coming in a future release (FEAT-005).")
			return nil
		},
	}
}
