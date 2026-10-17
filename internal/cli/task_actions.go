package cli

import (
	"context"
	"fmt"
	"strings"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/spf13/cobra"
)

// newTaskGroupCmd returns `termino task list|move|assign|done|sprint`. Tasks are given as
// KEY-N, N (current project) or a task ID; columns and members by name or ID.
func newTaskGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "List, move, assign, finish or plan tasks without opening the TUI",
	}
	cmd.AddCommand(newTaskListCmd("list"), newTaskMoveCmd(), newTaskAssignCmd(), newTaskDoneCmd(), newTaskSprintCmd())
	return cmd
}

// taskRef formats a task as KEY-N.
func taskRef(keys map[string]string, t taskdomain.Task) string {
	return fmt.Sprintf("%s-%d", keys[t.ProjectID], t.Number)
}

// moveToColumn moves t to the end of col and, when col is the project's Done
// column, checks the box in the task's source file.
func moveToColumn(ctx context.Context, t taskdomain.Task, col boarddomain.Column, cols []boarddomain.Column) error {
	taskUC, err := globalContainer.TaskUseCases()
	if err != nil {
		return err
	}
	existing, err := taskUC.ListTasks(ctx, taskdomain.TaskFilter{ColumnID: col.ID})
	if err != nil {
		return fmt.Errorf("list column tasks: %w", err)
	}
	position := 1.0
	for _, e := range existing {
		if e.ID != t.ID && e.Position >= position {
			position = e.Position + 1
		}
	}
	if err := taskUC.MoveTask(ctx, cliActor(), t.ID, col.ID, position); err != nil {
		return err
	}
	if done, ok := boarddomain.DoneColumn(cols); ok && done.ID == col.ID && t.SourcePath != nil {
		syncSvc, err := newSyncService()
		if err != nil {
			return err
		}
		if err := syncSvc.ExportTask(ctx, t.ID, taskSources()); err != nil {
			return fmt.Errorf("task moved, but checking its source box failed: %w", err)
		}
	}
	return nil
}

func newTaskMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <task> <column>",
		Short: "Move a task to a column",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			t, err := resolveTask(ctx, args[0])
			if err != nil {
				return err
			}
			col, cols, err := resolveBoardColumn(ctx, t.ProjectID, args[1])
			if err != nil {
				return err
			}
			if err := moveToColumn(ctx, t, col, cols); err != nil {
				return err
			}
			keys, _ := projectKeys(ctx)
			fmt.Printf("✓ %s moved to %s\n", taskRef(keys, t), sanitize.Line(col.Name))
			return nil
		},
	}
}

func newTaskDoneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "done <task>",
		Short: "Move a task to the Done column",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			t, err := resolveTask(ctx, args[0])
			if err != nil {
				return err
			}
			boardUC, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			board, err := boardUC.GetBoard(ctx, t.ProjectID)
			if err != nil {
				return fmt.Errorf("get board: %w", err)
			}
			done, ok := boarddomain.DoneColumn(board.Columns)
			if !ok {
				return fmt.Errorf("the project has no columns")
			}
			if err := moveToColumn(ctx, t, done, board.Columns); err != nil {
				return err
			}
			keys, _ := projectKeys(ctx)
			fmt.Printf("✓ %s done\n", taskRef(keys, t))
			return nil
		},
	}
}

func newTaskSprintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sprint <task> <sprint>",
		Short: "Put a task in a sprint",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return putTaskInSprint(context.Background(), args[0], args[1])
		},
	}
}

func newTaskAssignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "assign <task> <member|none>",
		Short: "Assign a task to a member, or unassign it with none",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			t, err := resolveTask(ctx, args[0])
			if err != nil {
				return err
			}
			var assignee *string
			to := "nobody"
			if !strings.EqualFold(strings.TrimSpace(args[1]), "none") {
				m, err := resolveMember(ctx, args[1])
				if err != nil {
					return err
				}
				assignee = &m.ID
				to = sanitize.Line(m.Name)
			}
			taskUC, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}
			if err := taskUC.AssignTask(ctx, cliActor(), t.ID, assignee); err != nil {
				return err
			}
			keys, _ := projectKeys(ctx)
			fmt.Printf("✓ %s assigned to %s\n", taskRef(keys, t), to)
			return nil
		},
	}
}

// newMyTasksCmd returns `termino my-tasks`: tasks assigned to the current
// member across every project (PRD 5.5).
func newMyTasksCmd() *cobra.Command {
	var memberRef string
	cmd := &cobra.Command{
		Use:   "my-tasks",
		Short: "List the tasks assigned to you in every project",
		Long: `List the tasks assigned to the current member in every project.
The current member is the one named by user_name in config.toml, or the
built-in "cli" member; --member picks another member by name or ID.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			m, err := currentMember(ctx, memberRef)
			if err != nil {
				return err
			}
			taskUC, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}
			tasks, err := taskUC.MyTasks(ctx, m.ID)
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				fmt.Printf("No tasks assigned to %s.\n", sanitize.Line(m.Name))
				return nil
			}
			keys, err := projectKeys(ctx)
			if err != nil {
				return err
			}
			columns, err := columnNames(ctx, tasks)
			if err != nil {
				return err
			}
			for _, t := range tasks {
				if _, active := keys[t.ProjectID]; !active {
					continue // archived project
				}
				fmt.Printf("  %s\t%s\t[%s]\t%s\n", taskRef(keys, t), sanitize.Line(t.Title),
					sanitize.Line(columns[t.ColumnID]), t.Priority)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&memberRef, "member", "", "Member name or ID (default: the current member)")
	return cmd
}

// currentMember resolves --member, else config user_name, else "cli".
func currentMember(ctx context.Context, ref string) (memberdomain.Member, error) {
	if strings.TrimSpace(ref) != "" {
		return resolveMember(ctx, ref)
	}
	cfg, err := globalContainer.Config()
	if err != nil {
		return memberdomain.Member{}, err
	}
	if cfg.UserName != "" {
		if m, err := resolveMember(ctx, cfg.UserName); err == nil {
			return m, nil
		}
	}
	return resolveMember(ctx, "cli")
}

// columnNames maps the column IDs of the given tasks to their names.
func columnNames(ctx context.Context, tasks []taskdomain.Task) (map[string]string, error) {
	boardUC, err := globalContainer.BoardUseCases()
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	seen := map[string]bool{}
	for _, t := range tasks {
		if seen[t.ProjectID] {
			continue
		}
		seen[t.ProjectID] = true
		board, err := boardUC.GetBoard(ctx, t.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("get board: %w", err)
		}
		for _, c := range board.Columns {
			names[c.ID] = c.Name
		}
	}
	return names, nil
}
