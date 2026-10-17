package cli

import (
	"context"

	apptui "github.com/lesquel/termino/internal/app/tui"
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
	// Open the project associated with the working directory, if any.
	if projectID, err := currentProjectID(); err == nil {
		cfg.CurrentProjectID = projectID
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

	// Two-way task-source sync: watch the files this project imported and
	// check a card's box when the board moves it to Done. Logs go to file.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sources := taskSources()
	actor := cliActor()
	syncSvc, err := newSyncService()
	if err != nil {
		return err
	}
	logger := fileLogger()
	watchDone := closedChan()
	if cfg.CurrentProjectID != "" {
		if done, werr := syncSvc.WatchImported(ctx, actor, cfg.CurrentProjectID, sources); werr != nil {
			logger.Warn("tui: task-source watcher not started", "err", werr)
		} else {
			watchDone = done
		}
	}
	onDone := func(taskID string) {
		if err := syncSvc.ExportTask(ctx, taskID, sources); err != nil {
			logger.Warn("tui: export to task source failed", "task", taskID, "err", err)
		}
	}

	deps := apptui.RootDeps{
		Config:     cfg,
		DB:         db,
		ProjectUC:  projectUC,
		BoardUC:    boardUC,
		TaskUC:     taskUC,
		AgentUC:    agentUC,
		MemberUC:   memberUC,
		Theme:      theme,
		OnTaskDone: onDone,
	}
	runErr := apptui.Start(deps)
	cancel()
	<-watchDone
	return runErr
}

// closedChan returns an already-closed channel.
func closedChan() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
