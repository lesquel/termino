package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newDoctorCmd returns `termino doctor` — stub for FEAT-005.
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose your Termino installation — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Doctor coming in a future release (FEAT-005).")
			return nil
		},
	}
}
