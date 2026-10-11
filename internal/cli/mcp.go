package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newMCPCmd returns `termino mcp` — stub for FEAT-004.
func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP server — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("MCP server coming in a future release (FEAT-004).")
			return nil
		},
	}
}
