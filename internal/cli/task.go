package cli

import (
	"context"
	"fmt"
	"strings"

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
		labels      []string
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

			labelNames, err := cleanLabels(labels)
			if err != nil {
				return err
			}

			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			key, err := projectKey(ctx, projectID)
			if err != nil {
				return err
			}

			// Resolve column: use the flag or fall back to the first column.
			colID, err := resolveColumn(projectID, columnFlag)
			if err != nil {
				return err
			}

			uc, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}

			actor := cliActor()
			t, err := uc.CreateTask(ctx, actor, taskapp.CreateTaskInput{
				ProjectID:   projectID,
				Title:       title,
				Description: desc,
				ColumnID:    colID,
				Priority:    prio,
			})
			if err != nil {
				return fmt.Errorf("create task: %w", err)
			}

			suffix := ""
			if len(labelNames) > 0 {
				if err := uc.AddLabels(ctx, actor, t.ID, labelNames); err != nil {
					return fmt.Errorf("task %s-%d created, but labelling failed: %w", key, t.Number, err)
				}
				if labelled, err := uc.FindTaskByID(ctx, t.ID); err == nil && len(labelled.Labels) > 0 {
					suffix = " [" + strings.Join(sanitizeAll(labelled.Labels), ", ") + "]"
				}
			}

			fmt.Printf("Created task %s-%d: %s%s\n", key, t.Number, sanitize.Line(t.Title), suffix)
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Task description")
	cmd.Flags().StringVar(&priority, "priority", "medium", "Priority: low, medium, high, critical")
	cmd.Flags().StringVar(&columnFlag, "column", "", "Column ID or name (defaults to first column)")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "Label to attach (repeatable or comma-separated; created when missing)")
	return cmd
}

// cleanLabels sanitizes label names typed by the user, dropping empty ones.
func cleanLabels(raw []string) ([]string, error) {
	var out []string
	for _, r := range raw {
		name := sanitize.Line(r)
		if name == "" {
			if strings.TrimSpace(r) != "" {
				return nil, fmt.Errorf("label %q is empty after removing control characters", r)
			}
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// sanitizeAll returns names made safe for terminal output.
func sanitizeAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = sanitize.Line(n)
	}
	return out
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

// resolveColumn returns the column ID to use for a new task: the column
// named or identified by colFlag, otherwise the project's first column.
func resolveColumn(projectID, colFlag string) (string, error) {
	if strings.TrimSpace(colFlag) != "" {
		col, _, err := resolveBoardColumn(context.Background(), projectID, colFlag)
		if err != nil {
			return "", err
		}
		return col.ID, nil
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
