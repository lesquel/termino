// Package domain defines the member aggregate, its value objects, and the
// repository interface. No imports outside the Go standard library are used.
package domain

import (
	"context"
	"time"
)

// Kind classifies a member as a human user or an AI agent.
type Kind string

const (
	KindHuman Kind = "human"
	KindAgent Kind = "agent"
)

// Member represents a participant in the workspace, either a human or an agent.
type Member struct {
	ID        string
	Name      string
	Kind      Kind
	CreatedAt time.Time
}

// MemberRepository defines the persistence contract for Member aggregates.
type MemberRepository interface {
	// Save inserts or updates the given Member.
	Save(ctx context.Context, m Member) error
	// FindByID retrieves a Member by its unique identifier.
	FindByID(ctx context.Context, id string) (Member, error)
	// FindByName retrieves a Member by name (case-sensitive).
	FindByName(ctx context.Context, name string) (Member, error)
	// List returns every Member in the workspace.
	List(ctx context.Context) ([]Member, error)
	// Delete permanently removes a Member by ID.
	Delete(ctx context.Context, id string) error
}
