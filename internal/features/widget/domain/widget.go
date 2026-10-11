// Package domain defines the widget summary value objects and repository
// interface. No imports outside the Go standard library are used.
package domain

import (
	"context"
	"time"
)

// TaskSummary is a compact representation of a task for use in the widget.
type TaskSummary struct {
	ID         string
	Ref        string // e.g. "WEB-42"
	Title      string
	DueDate    *time.Time
	ColumnName string
}

// Summary is the data model surfaced by the status-bar / tmux widget.
type Summary struct {
	ProjectName     string
	ProjectKey      string
	AssignedTasks   []TaskSummary
	InProgressTasks []TaskSummary
	DueSoonTasks    []TaskSummary
}

// WidgetRepository assembles the Summary from the underlying data stores.
type WidgetRepository interface {
	// GetSummary returns the widget summary for the given project and actor.
	GetSummary(ctx context.Context, projectID, actorID string) (Summary, error)
}
