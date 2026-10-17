package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/spf13/cobra"
)

// taskListFlags narrow `termino task list`.
type taskListFlags struct {
	column   string
	assignee string
	label    string
	priority string
}

// newLsCmd returns `termino ls`, a shortcut for `termino task list`.
func newLsCmd() *cobra.Command {
	cmd := newTaskListCmd("ls")
	cmd.Short = "List the tasks of the current project (same as task list)"
	return cmd
}

// newTaskListCmd returns the task listing command named use (list or ls).
func newTaskListCmd(use string) *cobra.Command {
	var f taskListFlags
	cmd := &cobra.Command{
		Use:   use,
		Short: "List the tasks of the current project, column by column",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listTasks(context.Background(), f)
		},
	}
	if use == "list" {
		cmd.Aliases = []string{"ls"}
	}
	cmd.Flags().StringVar(&f.column, "column", "", "Only this column (name or ID)")
	cmd.Flags().StringVar(&f.assignee, "assignee", "", "Only tasks assigned to this member (name or ID; none = unassigned)")
	cmd.Flags().StringVar(&f.label, "label", "", "Only tasks with this label")
	cmd.Flags().StringVar(&f.priority, "priority", "", "Only this priority: low, medium, high, critical")
	return cmd
}

func listTasks(ctx context.Context, f taskListFlags) error {
	var prio taskdomain.Priority
	if p := strings.ToLower(strings.TrimSpace(f.priority)); p != "" {
		prio = taskdomain.Priority(p)
		if !prio.Valid() {
			return fmt.Errorf("invalid priority %q (use low, medium, high or critical)", f.priority)
		}
	}
	projectID, err := currentProjectID()
	if err != nil {
		return err
	}
	key, err := projectKey(ctx, projectID)
	if err != nil {
		return err
	}
	boardUC, err := globalContainer.BoardUseCases()
	if err != nil {
		return err
	}
	board, err := boardUC.GetBoard(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get board: %w", err)
	}
	columns := board.Columns
	if strings.TrimSpace(f.column) != "" {
		col, _, err := resolveBoardColumn(ctx, projectID, f.column)
		if err != nil {
			return err
		}
		columns = []boarddomain.Column{col}
	}

	filter := taskdomain.TaskFilter{ProjectID: projectID}
	unassigned := strings.EqualFold(strings.TrimSpace(f.assignee), "none")
	if a := strings.TrimSpace(f.assignee); a != "" && !unassigned {
		m, err := resolveMember(ctx, a)
		if err != nil {
			return err
		}
		filter.AssigneeID = m.ID
	}

	taskUC, err := globalContainer.TaskUseCases()
	if err != nil {
		return err
	}
	tasks, err := taskUC.ListTasks(ctx, filter)
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	if len(tasks) == 0 && f == (taskListFlags{}) {
		fmt.Printf("No tasks in %s yet — add one with `termino add <title>`.\n", key)
		return nil
	}
	names := memberNames(ctx)

	byColumn := map[string][]taskdomain.Task{}
	for _, t := range tasks {
		if t.DeletedAt != nil || !matchesListFilter(t, f.label, prio, unassigned) {
			continue
		}
		byColumn[t.ColumnID] = append(byColumn[t.ColumnID], t)
	}
	printed := 0
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, col := range columns {
		for _, t := range byColumn[col.ID] {
			fmt.Fprintln(w, formatTaskLine(key, t, col.Name, names))
			printed++
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if printed == 0 {
		fmt.Println("No tasks match.")
	}
	return nil
}

// matchesListFilter applies the filters the repository does not.
func matchesListFilter(t taskdomain.Task, label string, prio taskdomain.Priority, unassigned bool) bool {
	if unassigned && t.AssigneeID != nil {
		return false
	}
	if prio != "" && t.Priority != prio {
		return false
	}
	if label = strings.TrimSpace(label); label != "" {
		for _, l := range t.Labels {
			if strings.EqualFold(l, label) {
				return true
			}
		}
		return false
	}
	return true
}

// formatTaskLine renders the tab-separated cells "  KEY-N  Title  [Column]
// priority  @assignee  labels" for a tabwriter. The assignee cell is always
// present (empty when unassigned) so labels line up; trailing empty cells
// are dropped so lines carry no trailing spaces.
func formatTaskLine(key string, t taskdomain.Task, column string, members map[string]string) string {
	assignee := ""
	if t.AssigneeID != nil {
		name := members[*t.AssigneeID]
		if name == "" {
			name = *t.AssigneeID
		}
		assignee = "@" + sanitize.Line(name)
	}
	parts := []string{
		fmt.Sprintf("  %s-%d", key, t.Number),
		sanitize.Line(t.Title),
		"[" + sanitize.Line(column) + "]",
		string(t.Priority),
		assignee,
		strings.Join(sanitizeAll(t.Labels), ","),
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "\t")
}

// memberNames maps member IDs to names; an error yields an empty map.
func memberNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	uc, err := globalContainer.MemberUseCases()
	if err != nil {
		return names
	}
	members, err := uc.ListMembers(ctx)
	if err != nil {
		return names
	}
	for _, m := range members {
		names[m.ID] = m.Name
	}
	return names
}
