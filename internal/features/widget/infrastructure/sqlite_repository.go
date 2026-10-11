// Package infrastructure provides SQLite-backed implementations of the widget
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// WidgetRepository is the SQLite-backed implementation of
// widgetdomain.WidgetRepository. It is intended to use a read-only DB connection.
type WidgetRepository struct {
	db *sql.DB
}

// NewWidgetRepository constructs a WidgetRepository backed by db.
func NewWidgetRepository(db *sql.DB) *WidgetRepository {
	return &WidgetRepository{db: db}
}

// GetSummary returns the widget summary for the given project and actor.
func (r *WidgetRepository) GetSummary(ctx context.Context, projectID, actorID string) (widgetdomain.Summary, error) {
	// Fetch project name and key.
	var projectName, projectKey string
	err := r.db.QueryRowContext(ctx, `
		SELECT name, key FROM projects WHERE id = ? AND archived_at IS NULL`, projectID).
		Scan(&projectName, &projectKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return widgetdomain.Summary{}, fmt.Errorf("project not found: %s", projectID)
		}
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary fetch project: %w", err)
	}

	// Assigned tasks (tasks assigned to the actor).
	assigned, err := r.fetchTaskSummaries(ctx, `
		SELECT t.id,
		       ? || '-' || t.number,
		       t.title,
		       t.due_date,
		       c.name
		FROM   tasks t
		JOIN   columns c ON c.id = t.column_id
		WHERE  t.assignee_id = ?
		  AND  t.project_id  = ?
		  AND  t.deleted_at  IS NULL`, projectKey, actorID, projectID)
	if err != nil {
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary assigned tasks: %w", err)
	}

	// In-progress tasks: columns whose name contains 'progress' (case-insensitive)
	// or whose position = 1 (middle column heuristic).
	inProgress, err := r.fetchTaskSummaries(ctx, `
		SELECT t.id,
		       ? || '-' || t.number,
		       t.title,
		       t.due_date,
		       c.name
		FROM   tasks t
		JOIN   columns c ON c.id = t.column_id
		WHERE  t.project_id = ?
		  AND  t.deleted_at IS NULL
		  AND  (LOWER(c.name) LIKE '%progress%' OR c.position = 1)`, projectKey, projectID)
	if err != nil {
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary in-progress tasks: %w", err)
	}

	// Due-soon tasks: due within the next 7 days and not deleted.
	now := time.Now().UTC()
	soon := now.Add(7 * 24 * time.Hour)
	dueSoon, err := r.fetchTaskSummaries(ctx, `
		SELECT t.id,
		       ? || '-' || t.number,
		       t.title,
		       t.due_date,
		       c.name
		FROM   tasks t
		JOIN   columns c ON c.id = t.column_id
		WHERE  t.project_id = ?
		  AND  t.deleted_at IS NULL
		  AND  t.due_date BETWEEN ? AND ?`,
		projectKey, projectID,
		now.Format(time.RFC3339),
		soon.Format(time.RFC3339),
	)
	if err != nil {
		return widgetdomain.Summary{}, fmt.Errorf("widget.GetSummary due-soon tasks: %w", err)
	}

	return widgetdomain.Summary{
		ProjectName:     projectName,
		ProjectKey:      projectKey,
		AssignedTasks:   assigned,
		InProgressTasks: inProgress,
		DueSoonTasks:    dueSoon,
	}, nil
}

// fetchTaskSummaries runs a query that returns (id, ref, title, due_date, column_name) rows.
func (r *WidgetRepository) fetchTaskSummaries(ctx context.Context, query string, args ...any) ([]widgetdomain.TaskSummary, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		// Ignore "no such column" errors gracefully for empty projects.
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
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
		if dueDate != nil {
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
