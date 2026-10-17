package cli

import (
	apptui "github.com/lesquel/termino/internal/app/tui"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/spf13/cobra"
)

// newBoardCmd returns `termino board` — opens the TUI board.
func newBoardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "Open the interactive Kanban board (TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return startTUI()
		},
	}
}

// startTUI builds TUI dependencies and launches the program.
func startTUI() error {
	cfg, err := globalContainer.Config()
	if err != nil {
		return err
	}
	db, err := globalContainer.DB()
	if err != nil {
		return err
	}
	projectUC, err := globalContainer.ProjectUseCases()
	if err != nil {
		return err
	}
	boardUC, err := globalContainer.BoardUseCases()
	if err != nil {
		return err
	}
	taskUC, err := globalContainer.TaskUseCases()
	if err != nil {
		return err
	}
	agentUC, err := globalContainer.AgentUseCases()
	if err != nil {
		return err
	}
	memberUC, err := globalContainer.MemberUseCases()
	if err != nil {
		return err
	}

	theme, err := apptui.Load(cfg.Theme)
	if err != nil {
		// Fall back gracefully with nil theme — root model handles nil.
		theme = nil
	}

	deps := apptui.RootDeps{
		Config:    cfg,
		DB:        db,
		ProjectUC: projectUC,
		BoardUC:   boardUC,
		TaskUC:    taskUC,
		AgentUC:   agentUC,
		MemberUC:  memberUC,
		Theme:     theme,
	}
	return apptui.Start(deps)
}

// currentProjectName returns the current project name, or empty string.
func currentProjectName(cfg *config.Config) string {
	return cfg.CurrentProjectID
}
