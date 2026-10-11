// Package cli wires the Cobra command tree for the termino binary.
// The full command tree is implemented in FEAT-003; this stub ensures
// `go build ./...` succeeds from the first commit.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "termino",
	Short: "Termino — local task board with TUI and MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("termino: not yet implemented")
		return nil
	},
}

// Execute is the entry point called by main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
