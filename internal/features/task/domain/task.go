// Package domain defines the task aggregate and its supporting types.
// No imports outside the Go standard library are used.
package domain

import (
	"context"
	"time"
)

// Priority represents the urgency level of a task.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// Task is the central aggregate of the application.
type Task struct {
	ID          string
	ProjectID   string
	Number      int     // monotonically increasing per project
	Title       string
	Description string
	ColumnID    string
	SprintID    *string
	Priority    Priority
	AssigneeID  *string
	DueDate     *time.Time
	Points      *int
	Position    float64 // fractional position within the column
	DeletedAt   *time.Time
	Source      *string // name of the source adapter (e.g. "kirospec")
	SourcePath  *string // filesystem path of the source file
	SourceRef   *string // unique reference within the source file
}

// Label is a coloured tag that can be applied to multiple tasks.
type Label struct {
	ID        string
	ProjectID string
	Name      string
	Color     string
}

// Comment is a textual note attached to a task by a member.
type Comment struct {
	ID        string
	TaskID    string
	AuthorID  string
	Body      string
	CreatedAt time.Time
}

// Activity is an immutable audit-trail entry for changes made to a task.
type Activity struct {
	ID        string
	TaskID    string
	ActorID   string
	Action    string
	Payload   map[string]any
	CreatedAt time.Time
}

// TaskFilter holds optional predicates for listing tasks.
type TaskFilter struct {
	ProjectID      string
	ColumnID       string
	AssigneeID     string
	SprintID       string
	LabelIDs       []string
	SearchText     string
	IncludeDeleted bool
}

// TaskRepository defines the persistence contract for Task aggregates.
type TaskRepository interface {
	// Save inserts or updates the given Task.
	Save(ctx context.Context, t Task) error
	// FindByID retrieves a Task by its unique identifier.
	FindByID(ctx context.Context, id string) (Task, error)
	// FindByNumber retrieves a Task by its human-readable project-scoped number.
	FindByNumber(ctx context.Context, projectID string, number int) (Task, error)
	// List returns tasks matching the supplied filter.
	List(ctx context.Context, filter TaskFilter) ([]Task, error)
	// SoftDelete sets DeletedAt on the task.
	SoftDelete(ctx context.Context, id string, at time.Time) error
	// Restore clears DeletedAt on the task.
	Restore(ctx context.Context, id string) error
	// NextNumber returns the next available task number for the project.
	NextNumber(ctx context.Context, projectID string) (int, error)
}

// CommentRepository defines persistence for task comments.
type CommentRepository interface {
	// Save inserts or updates the given Comment.
	Save(ctx context.Context, c Comment) error
	// ListByTask returns all comments for the given task ID.
	ListByTask(ctx context.Context, taskID string) ([]Comment, error)
}

// ActivityRepository is an append-only store for activity log entries.
// Entries are never updated or deleted.
type ActivityRepository interface {
	// Append persists a new Activity entry.
	Append(ctx context.Context, a Activity) error
	// ListByTask returns all activity entries for a task, oldest first.
	ListByTask(ctx context.Context, taskID string) ([]Activity, error)
	// ListRecent returns the most recent limit entries across a project.
	ListRecent(ctx context.Context, projectID string, limit int) ([]Activity, error)
}
