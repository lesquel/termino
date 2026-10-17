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

// ColumnCount is the number of live tasks in one board column.
type ColumnCount struct {
	Name  string
	Count int
}

// Summary is the data model surfaced by the shell widget.
type Summary struct {
	ProjectName string
	ProjectKey  string
	// Columns lists every board column in board order with its task count.
	Columns []ColumnCount
	// AssignedTasks are the open tasks assigned to the actor.
	AssignedTasks []TaskSummary
	// InProgressTasks are the tasks in the in-progress column(s).
	InProgressTasks []TaskSummary
	// DueSoonTasks are open tasks due within a week, overdue ones included.
	DueSoonTasks []TaskSummary
	// NextTasks are the first open tasks of the first column; filled only
	// when nothing is in progress or assigned, so the widget is never empty
	// while the project has open work.
	NextTasks []TaskSummary
}

// TotalTasks is the number of live tasks across all columns.
func (s Summary) TotalTasks() int {
	total := 0
	for _, c := range s.Columns {
		total += c.Count
	}
	return total
}

// WidgetRepository assembles the Summary from the underlying data stores.
type WidgetRepository interface {
	// GetSummary returns the widget summary for the given project and actor.
	// actor is a member ID or name.
	GetSummary(ctx context.Context, projectID, actor string) (Summary, error)
}
