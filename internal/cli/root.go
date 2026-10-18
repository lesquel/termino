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
	// Every command logs to $XDG_STATE_HOME/termino/termino.log, never to
	// stdout/stderr: that would corrupt the TUI and the MCP stdio protocol.
	// `--help` does not run this hook, so it still touches no files.
	// A shared project imports its .termino files before any command runs.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		fileLogger()
		return syncSharedOnStart(cmd)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Running `termino` with no subcommand opens the TUI.
		return startTUI(cmd)
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
		newTaskGroupCmd(),
		newLsCmd(),
		newMyTasksCmd(),
		newMemberCmd(),
		newSprintCmd(),
		newColumnCmd(),
		newBoardCmd(),
		newWidgetCmd(),
		newMCPCmd(),
		newImportCmd(),
		newConnectCmd(),
		newDisconnectCmd(),
		newSetupCmd(),
		newUninstallCmd(),
		newDoctorCmd(),
		newThemeCmd(),
		newShareCmd(),
		newVersionCmd(),
	)
	// Suppress Cobra's default "Error:" prefix — our commands handle their own output.
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	// Usage is printed for argument and flag mistakes only, as one line.
	rootCmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return withUsage(c, err)
	})
	strictArgs(rootCmd)
}

// withUsage appends the command's usage line to an argument or flag error.
func withUsage(c *cobra.Command, err error) error {
	return fmt.Errorf("%w\nUsage: %s", err, c.UseLine())
}

// strictArgs walks the command tree: argument errors gain the usage line,
// and a group command given an unknown subcommand fails (with suggestions)
// instead of printing its help and exiting 0.
func strictArgs(c *cobra.Command) {
	for _, sub := range c.Commands() {
		strictArgs(sub)
	}
	if !c.HasParent() {
		return // cobra already rejects unknown root subcommands
	}
	switch {
	case c.Args != nil:
		validate := c.Args
		c.Args = func(cmd *cobra.Command, args []string) error {
			if err := validate(cmd, args); err != nil {
				return withUsage(cmd, err)
			}
			return nil
		}
	case c.HasSubCommands():
		c.Args = unknownSubcommand
		if !c.Runnable() {
			// Cobra skips argument checks on commands that cannot run.
			c.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
		}
	}
}

// unknownSubcommand rejects positional arguments on a group command, naming
// the closest subcommands.
func unknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2 // cobra only defaults this on the root
	}
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?"
		for _, s := range suggestions {
			msg += "\n\t" + s
		}
	}
	return fmt.Errorf("%s\n\nRun '%s --help' for usage", msg, cmd.CommandPath())
}

// fatalErr prints an error message to stderr and exits with code 1.
func fatalErr(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
