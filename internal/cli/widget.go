package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// newWidgetCmd returns `termino widget` and its subcommands.
func newWidgetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "widget",
		Short: "Display the status-bar widget summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := globalContainer.Config()
			if err != nil {
				return err
			}
			if cfg.CurrentProjectID == "" {
				return fmt.Errorf("no current project set — run `termino project new` first")
			}

			uc, err := globalContainer.WidgetUseCases()
			if err != nil {
				return err
			}
			summary, err := uc.GetSummary(context.Background(), cfg.CurrentProjectID, "cli", "")
			if err != nil {
				return fmt.Errorf("get summary: %w", err)
			}

			fmt.Printf("%s (%s)\n", summary.ProjectName, summary.ProjectKey)
			fmt.Printf("  Assigned:    %d\n", len(summary.AssignedTasks))
			fmt.Printf("  In Progress: %d\n", len(summary.InProgressTasks))
			fmt.Printf("  Due Soon:    %d\n", len(summary.DueSoonTasks))
			return nil
		},
	}

	cmd.AddCommand(newWidgetInstallCmd(), newWidgetUninstallCmd())
	return cmd
}

func newWidgetInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the tmux/shell widget — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Widget install coming in a future release (FEAT-005).")
			return nil
		},
	}
}

func newWidgetUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the tmux/shell widget — coming soon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Widget uninstall coming in a future release (FEAT-005).")
			return nil
		},
	}
}
