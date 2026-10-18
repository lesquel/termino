package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/x/term"
	apptui "github.com/lesquel/termino/internal/app/tui"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/spf13/cobra"
)

// newBoardCmd returns `termino board` — opens the TUI board.
func newBoardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "Open the interactive Kanban board (TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return startTUI(cmd)
		},
	}
}

// startTUI builds TUI dependencies and launches the program. Before any
// project exists it runs the setup wizard first (or, without a terminal,
// says how to set up) instead of opening an empty board.
func startTUI(cmd *cobra.Command) error {
	if ready, err := ensureProject(cmd); err != nil || !ready {
		return err
	}
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
	// Shared projects re-import their .termino files when they change.
	share, shareDone := watchShared(ctx, cfg.CurrentProjectID)
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
		Settings:   settingsDeps(),
		Share:      apptui.ShareDeps{Version: share.Version, Warning: share.Warning},
	}
	runErr := apptui.Start(deps)
	cancel()
	<-watchDone
	<-shareDone
	return runErr
}

// settingsDeps wires the board's settings screen: the available themes and
// the shell widget for $SHELL, toggled through the same use case as
// `termino widget install/uninstall`.
func settingsDeps() apptui.SettingsDeps {
	home, _ := os.UserHomeDir()
	registry := setupdomain.NewInstallationRegistry(filepath.Join(config.StateDir(), "installed.json"))
	return apptui.SettingsDeps{
		Themes: availableThemes(),
		Widget: setupapp.NewShellWidget(registry, os.Getenv("SHELL"), home),
		Home:   home,
	}
}

// ensureProject reports whether a project exists, running the setup wizard
// when none does and a terminal is attached. Cancelling setup returns false.
func ensureProject(cmd *cobra.Command) (bool, error) {
	projectUC, err := globalContainer.ProjectUseCases()
	if err != nil {
		return false, err
	}
	projects, err := projectUC.ListProjects(context.Background())
	if err != nil {
		return false, err
	}
	if len(projects) > 0 {
		return true, nil
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(cmd.OutOrStdout(), "No projects yet. Run `termino setup` to create your first project.")
		return false, nil
	}
	if err := runSetup(cmd, &setupFlags{openingBoard: true}); err != nil {
		return false, err
	}
	projects, err = projectUC.ListProjects(context.Background())
	if err != nil || len(projects) == 0 {
		return false, err
	}
	globalContainer.ReloadConfig() // setup wrote the user and current project
	return true, nil
}

// closedChan returns an already-closed channel.
func closedChan() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
