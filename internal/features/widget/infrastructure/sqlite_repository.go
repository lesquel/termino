// Package infrastructure provides SQLite-backed implementations of the widget
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// listLimit caps every task list query; renderers show fewer.
const listLimit = 5

// WidgetRepository is the SQLite-backed implementation of
// widgetdomain.WidgetRepository. It is intended to use a read-only DB connection.
type WidgetRepository struct {
	db *sql.DB
}

// NewWidgetRepository constructs a WidgetRepository backed by db.
func NewWidgetRepository(db *sql.DB) *WidgetRepository {
	return &WidgetRepository{db: db}
}

// column is one board column with its live task count.
type column struct {
	id       string
	name     string
	position float64
	count    int
}

// GetSummary returns the widget summary for the given project and actor
// (a member ID or name).
func (r *WidgetRepository) GetSummary(ctx context.Context, projectID, actor string) (widgetdomain.Summary, error) {
	var projectName, projectKey string
	err := r.db.QueryRowContext(ctx, `
		SELECT name, key FROM projects WHERE id = ? AND archived_at IS NULL`, projectID).
		Scan(&projectName, &projectKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return widgetdomain.Summary{}, fmt.Errorf("project not found: %s", projectID)
		}
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary fetch project: %w", err)
	}

	cols, err := r.columns(ctx, projectID)
	if err != nil {
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary columns: %w", err)
	}
	summary := widgetdomain.Summary{ProjectName: projectName, ProjectKey: projectKey}
	for _, c := range cols {
		summary.Columns = append(summary.Columns, widgetdomain.ColumnCount{Name: c.name, Count: c.count})
	}
	if len(cols) == 0 {
		return summary, nil
	}
	doneID := doneColumnID(cols)
	open := openColumnIDs(cols, doneID)
	if len(open) == 0 {
		return summary, nil
	}

	inOpen, openArgs := inClause(open)
	base := `
		SELECT t.id, ? || '-' || t.number, t.title, t.due_date, c.name
		FROM   tasks t
		JOIN   columns c ON c.id = t.column_id
		WHERE  t.project_id = ? AND t.deleted_at IS NULL AND t.column_id IN ` + inOpen

	args := func(extra ...any) []any {
		a := append([]any{projectKey, projectID}, openArgs...)
		return append(a, extra...)
	}

	summary.AssignedTasks, err = r.fetchTaskSummaries(ctx, base+`
		  AND  t.assignee_id IN (SELECT id FROM members WHERE id = ? OR name = ?)
		ORDER BY c.position, t.position, t.number
		LIMIT `+fmt.Sprint(listLimit), args(actor, actor)...)
	if err != nil {
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary assigned tasks: %w", err)
	}

	if progress := progressColumnIDs(cols, doneID); len(progress) > 0 {
		inProg, progArgs := inClause(progress)
		summary.InProgressTasks, err = r.fetchTaskSummaries(ctx, base+`
			  AND  t.column_id IN `+inProg+`
			ORDER BY c.position, t.position, t.number
			LIMIT `+fmt.Sprint(listLimit), args(progArgs...)...)
		if err != nil {
			return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary in-progress tasks: %w", err)
		}
	}

	// Due soon: open tasks due within the next 7 days, overdue ones included.
	soon := time.Now().UTC().Add(7 * 24 * time.Hour)
	summary.DueSoonTasks, err = r.fetchTaskSummaries(ctx, base+`
		  AND  t.due_date IS NOT NULL AND t.due_date <= ?
		ORDER BY t.due_date, t.number
		LIMIT `+fmt.Sprint(listLimit), args(soon.Format(time.RFC3339))...)
	if err != nil {
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary due-soon tasks: %w", err)
	}

	// Nothing active: show what comes next, so open work is never hidden.
	if len(summary.AssignedTasks) == 0 && len(summary.InProgressTasks) == 0 {
		summary.NextTasks, err = r.fetchTaskSummaries(ctx, base+`
			  AND  t.column_id = ?
			ORDER BY t.position, t.number
			LIMIT `+fmt.Sprint(listLimit), args(open[0])...)
		if err != nil {
			return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary next tasks: %w", err)
		}
	}
	return summary, nil
}

// columns returns the project's columns in board order with live task counts.
func (r *WidgetRepository) columns(ctx context.Context, projectID string) ([]column, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.position, COUNT(t.id)
		FROM   columns c
		LEFT JOIN tasks t ON t.column_id = c.id AND t.deleted_at IS NULL
		WHERE  c.project_id = ?
		GROUP BY c.id
		ORDER BY c.position, c.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.id, &c.name, &c.position, &c.count); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// doneColumnID mirrors board.DoneColumn: the column named "Done" (any case),
// otherwise the last column. cols must be non-empty and in board order.
func doneColumnID(cols []column) string {
	for _, c := range cols {
		if strings.EqualFold(strings.TrimSpace(c.name), "done") {
			return c.id
		}
	}
	return cols[len(cols)-1].id
}

// openColumnIDs returns every column except the done one, in board order.
func openColumnIDs(cols []column, doneID string) []string {
	ids := make([]string, 0, len(cols))
	for _, c := range cols {
		if c.id != doneID {
			ids = append(ids, c.id)
		}
	}
	return ids
}

// progressColumnIDs returns the columns that hold work in flight: names with
// "progress", "doing" or "review"; failing that, the second column when it
// is not the done column.
func progressColumnIDs(cols []column, doneID string) []string {
	var ids []string
	for _, c := range cols {
		name := strings.ToLower(c.name)
		if c.id != doneID && (strings.Contains(name, "progress") || strings.Contains(name, "doing") || strings.Contains(name, "review")) {
			ids = append(ids, c.id)
		}
	}
	if len(ids) == 0 && len(cols) > 2 && cols[1].id != doneID {
		ids = append(ids, cols[1].id)
	}
	return ids
}

// inClause renders "(?, ?, …)" for ids and returns the matching args.
func inClause(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return "(" + strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ") + ")", args
}

// fetchTaskSummaries runs a query that returns (id, ref, title, due_date, column_name) rows.
func (r *WidgetRepository) fetchTaskSummaries(ctx context.Context, query string, args ...any) ([]widgetdomain.TaskSummary, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetchTaskSummaries: %w", err)
	}
	defer rows.Close()

	var summaries []widgetdomain.TaskSummary
	for rows.Next() {
		var ts widgetdomain.TaskSummary
		var dueDate *string
		if err := rows.Scan(&ts.ID, &ts.Ref, &ts.Title, &dueDate, &ts.ColumnName); err != nil {
			return nil, fmt.Errorf("task summary scan: %w", err)
		}
		if dueDate != nil && *dueDate != "" {
			dd, err := time.Parse(time.RFC3339, *dueDate)
			if err != nil {
				return nil, fmt.Errorf("task summary parse due_date: %w", err)
			}
			ts.DueDate = &dd
		}
		summaries = append(summaries, ts)
	}
	return summaries, rows.Err()
}
