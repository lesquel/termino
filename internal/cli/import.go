package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newImportCmd returns `termino import` — stub for FEAT-005.
func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import",
		Short: "Import tasks from source files — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Import coming in a future release (FEAT-005).")
			return nil
		},
	}
}
