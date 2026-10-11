// Package domain defines the sprint aggregate and repository interface.
// No imports outside the Go standard library are used.
package domain

import (
	"context"
	"errors"
	"time"
)

// SprintState represents the lifecycle state of a sprint.
type SprintState string

const (
	SprintStateDraft  SprintState = "draft"
	SprintStateActive SprintState = "active"
	SprintStateClosed SprintState = "closed"
)

// Sprint is a time-boxed iteration used in Scrum-style projects.
type Sprint struct {
	ID        string
	ProjectID string
	Name      string
	StartsAt  time.Time
	EndsAt    time.Time
	State     SprintState
}

// ErrActiveSprintExists is returned when attempting to start a sprint while
// another sprint is already active in the same project.
var ErrActiveSprintExists = errors.New("a sprint is already active in this project")

// SprintRepository defines the persistence contract for Sprint aggregates.
type SprintRepository interface {
	// Save inserts or updates the given Sprint.
	Save(ctx context.Context, s Sprint) error
	// FindByID retrieves a Sprint by its unique identifier.
	FindByID(ctx context.Context, id string) (Sprint, error)
	// FindActive returns the currently active sprint for a project.
	// Returns nil, nil when no active sprint exists.
	FindActive(ctx context.Context, projectID string) (*Sprint, error)
	// ListByProject returns all sprints for the given project.
	ListByProject(ctx context.Context, projectID string) ([]Sprint, error)
}
