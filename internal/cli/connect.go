package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newConnectCmd returns `termino connect` and `termino disconnect` — stubs for FEAT-005.
func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect a tool integration — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Connect coming in a future release (FEAT-005).")
			return nil
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "disconnect",
		Short: "Disconnect a tool integration — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Disconnect coming in a future release (FEAT-005).")
			return nil
		},
	})
	return cmd
}
