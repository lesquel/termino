package cli

import (
	"context"
	"fmt"

	sharedomain "github.com/lesquel/termino/internal/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/spf13/cobra"
)

// newTaskCmd returns the `termino add` command.
func newTaskCmd() *cobra.Command {
	var (
		description string
		priority    string
		columnFlag  string
	)

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task to the current project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]

			cfg, err := globalContainer.Config()
			if err != nil {
				return err
			}
			if cfg.CurrentProjectID == "" {
				return fmt.Errorf("no current project set — run `termino project new` first")
			}

			// Resolve column: use the flag or fall back to the first column.
			colID, err := resolveColumn(cfg.CurrentProjectID, columnFlag)
			if err != nil {
				return err
			}

			prio := taskdomain.Priority(priority)
			if prio == "" {
				prio = taskdomain.PriorityMedium
			}

			uc, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}

			actor := sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
			t, err := uc.CreateTask(context.Background(), actor, taskapp.CreateTaskInput{
				ProjectID:   cfg.CurrentProjectID,
				Title:       title,
				Description: description,
				ColumnID:    colID,
				Priority:    prio,
			})
			if err != nil {
				return fmt.Errorf("create task: %w", err)
			}

			// Fetch project key to build the task ref.
			projRepo, err := globalContainer.ProjectUseCases()
			if err != nil {
				return err
			}
			p, err := projRepo.ListProjects(context.Background())
			if err != nil {
				return err
			}
			key := "TER"
			for _, proj := range p {
				if proj.ID == cfg.CurrentProjectID {
					key = proj.Key
					break
				}
			}

			fmt.Printf("Created task %s-%d: %s\n", key, t.Number, t.Title)
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Task description")
	cmd.Flags().StringVar(&priority, "priority", "medium", "Priority: low, medium, high, critical")
	cmd.Flags().StringVar(&columnFlag, "column", "", "Column ID or name (defaults to first column)")
	return cmd
}

// resolveColumn returns the column ID to use for a new task.
// If colFlag is provided it is treated as the column ID directly.
// Otherwise, the first column of the project is used.
func resolveColumn(projectID, colFlag string) (string, error) {
	if colFlag != "" {
		return colFlag, nil
	}
	boardUC, err := globalContainer.BoardUseCases()
	if err != nil {
		return "", err
	}
	board, err := boardUC.GetBoard(context.Background(), projectID)
	if err != nil {
		return "", fmt.Errorf("get board: %w", err)
	}

	// Use projectdomain.Column helpers to find the lowest-position column.
	cols := board.Columns
	if len(cols) == 0 {
		return "", fmt.Errorf("project %s has no columns", projectID)
	}
	// Columns are sorted by position from the repository.
	_ = projectdomain.MethodologyKanban // imported for package linkage
	return cols[0].ID, nil
}
