package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the build version. Release builds override it with
// -ldflags "-X github.com/lesquel/termino/internal/cli.Version=<tag>".
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the termino version",
		Args:  cobra.NoArgs,
		// Printing the version must not create the log file.
		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "termino %s\n", Version)
			return err
		},
	}
}
