package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newBoardCmd returns `termino board` — opens the TUI board (stub for FEAT-004).
func newBoardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "Open the interactive board (TUI) — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("TUI board coming in a future release.")
			return nil
		},
	}
}
