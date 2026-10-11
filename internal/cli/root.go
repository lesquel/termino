// Package cli wires the Cobra command tree for the termino binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "termino",
	Short: "Termino — local task board with TUI and MCP server",
	Long: `Termino is a local-first task board that runs entirely on your machine.
It stores data in SQLite and exposes a TUI, MCP server, and CLI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Running `termino` with no subcommand opens the TUI.
		return startTUI()
	},
}

// Execute is the entry point called by main.go.
// It returns an error so the caller can set the exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(
		newProjectCmd(),
		newTaskCmd(),
		newBoardCmd(),
		newWidgetCmd(),
		newMCPCmd(),
		newImportCmd(),
		newConnectCmd(),
		newSetupCmd(),
		newUninstallCmd(),
		newDoctorCmd(),
		newThemeCmd(),
	)
	// Suppress Cobra's default "Error:" prefix — our commands handle their own output.
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
}

// fatalErr prints an error message to stderr and exits with code 1.
func fatalErr(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
