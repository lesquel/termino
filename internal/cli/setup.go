package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newSetupCmd returns `termino setup` — stub for FEAT-005.
func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up Termino integrations — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Setup coming in a future release (FEAT-005).")
			return nil
		},
	}
}
