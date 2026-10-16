// Package domain defines the project aggregate, methodology strategy, and
// repository interface. No imports outside the Go standard library are used.
package domain

import (
	"context"
	"time"
)

// MethodologyKind identifies the development methodology used by a project.
type MethodologyKind string

const (
	MethodologyKanban MethodologyKind = "kanban"
	MethodologyScrum  MethodologyKind = "scrum"
)

// ColumnTemplate is used by Methodology.DefaultColumns to describe the initial
// board column set for a new project.
type ColumnTemplate struct {
	Name     string
	Position int
}

// Task and Column are forward declarations used by the Methodology interface.
// They are intentionally minimal here; the full types live in their own domain
// packages, but the Methodology contract must be expressible without importing
// those packages (to avoid circular dependencies).
//
// Project domain only needs the method signatures; the concrete types are
// supplied by the application layer which imports both domains.

// Task is a minimal reference type used within Methodology to avoid importing
// the task domain package from the project domain package.
type Task struct {
	ID       string
	ColumnID string
}

// Column is a minimal reference type used within Methodology.
type Column struct {
	ID       string
	Name     string
	Position int
	WIPLimit *int
}

// Methodology is the strategy interface that encapsulates the rules for a
// particular development workflow (Kanban or Scrum).
type Methodology interface {
	// Kind returns the identifier for this methodology.
	Kind() MethodologyKind
	// DefaultColumns returns the initial column set to create when a project
	// is first created.
	DefaultColumns() []ColumnTemplate
	// CanMove validates whether a task may be moved to the target column on
	// the given board. Returns a non-nil error with a human-readable reason if
	// the move is not allowed.
	CanMove(task Task, to Column, columns []Column) error
}

// Project is the root aggregate for a workspace project.
type Project struct {
	ID             string
	Key            string // 2-5 uppercase letters, unique per workspace
	Name           string
	Methodology    MethodologyKind
	AssociatedPath string // filesystem path the widget uses to auto-detect this project (CWD); "" when unset
	CreatedAt      time.Time
	ArchivedAt     *time.Time
}

// ProjectRepository defines the persistence contract for Project aggregates.
type ProjectRepository interface {
	// Save inserts or updates the given Project.
	Save(ctx context.Context, p Project) error
	// FindByID retrieves a Project by its unique identifier.
	FindByID(ctx context.Context, id string) (Project, error)
	// FindByKey retrieves a Project by its short key (e.g. "WEB").
	FindByKey(ctx context.Context, key string) (Project, error)
	// List returns all non-archived projects.
	List(ctx context.Context) ([]Project, error)
	// Archive marks a project as archived by setting ArchivedAt.
	Archive(ctx context.Context, id string, at time.Time) error
}

// ProjectDetector resolves the project associated with a filesystem path.
// Used by the widget feature to auto-detect the current project from CWD.
type ProjectDetector interface {
	// DetectForPath returns the projectID whose configuration is rooted at or
	// above fsPath. Returns an error if no project can be detected.
	DetectForPath(fsPath string) (string, error)
}

// KanbanMethodology implements the Kanban workflow strategy.
type KanbanMethodology struct{}

// Kind implements Methodology.
func (KanbanMethodology) Kind() MethodologyKind { return MethodologyKanban }

// DefaultColumns returns the classic Kanban columns.
func (KanbanMethodology) DefaultColumns() []ColumnTemplate {
	return []ColumnTemplate{
		{Name: "Backlog", Position: 0},
		{Name: "In Progress", Position: 1},
		{Name: "Done", Position: 2},
	}
}

// CanMove implements Methodology. Kanban allows free movement between any columns.
func (KanbanMethodology) CanMove(_ Task, _ Column, _ []Column) error { return nil }

// ScrumMethodology implements the Scrum workflow strategy.
type ScrumMethodology struct{}

// Kind implements Methodology.
func (ScrumMethodology) Kind() MethodologyKind { return MethodologyScrum }

// DefaultColumns returns the typical Scrum board columns.
func (ScrumMethodology) DefaultColumns() []ColumnTemplate {
	return []ColumnTemplate{
		{Name: "Backlog", Position: 0},
		{Name: "Sprint Backlog", Position: 1},
		{Name: "In Progress", Position: 2},
		{Name: "Review", Position: 3},
		{Name: "Done", Position: 4},
	}
}

// CanMove implements Methodology. Scrum allows free movement between columns.
func (ScrumMethodology) CanMove(_ Task, _ Column, _ []Column) error { return nil }

// MethodologyFor returns the Methodology implementation for the given kind.
func MethodologyFor(kind MethodologyKind) Methodology {
	switch kind {
	case MethodologyScrum:
		return ScrumMethodology{}
	default:
		return KanbanMethodology{}
	}
}
