package cli

import (
	"context"
	"fmt"
	"strings"

	sharedomain "github.com/lesquel/termino/internal/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
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
			title, desc, prio, err := normalizeTaskInput(args[0], description, priority)
			if err != nil {
				return err
			}

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

			uc, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}

			actor := sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
			t, err := uc.CreateTask(context.Background(), actor, taskapp.CreateTaskInput{
				ProjectID:   cfg.CurrentProjectID,
				Title:       title,
				Description: desc,
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

			fmt.Printf("Created task %s-%d: %s\n", key, t.Number, sanitize.Line(t.Title))
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Task description")
	cmd.Flags().StringVar(&priority, "priority", "medium", "Priority: low, medium, high, critical")
	cmd.Flags().StringVar(&columnFlag, "column", "", "Column ID or name (defaults to first column)")
	return cmd
}

// normalizeTaskInput sanitizes user-supplied task text at the CLI edge and
// validates the priority against the closed enum (empty means medium).
func normalizeTaskInput(rawTitle, rawDesc, rawPriority string) (string, string, taskdomain.Priority, error) {
	title := sanitize.Line(rawTitle)
	if title == "" {
		return "", "", "", fmt.Errorf("title is empty after removing control characters")
	}
	prio := taskdomain.PriorityMedium
	if p := strings.ToLower(strings.TrimSpace(rawPriority)); p != "" {
		prio = taskdomain.Priority(p)
		if !prio.Valid() {
			return "", "", "", fmt.Errorf("invalid priority %q (use low, medium, high or critical)", rawPriority)
		}
	}
	return title, sanitize.Clean(rawDesc), prio, nil
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
