// Package domain defines the agent session aggregate and repository interface.
// No imports outside the Go standard library are used.
package domain

import (
	"context"
	"time"
)

// AgentSession tracks the presence and current task of a connected AI agent.
type AgentSession struct {
	AgentID       string
	AgentName     string
	LastSeenAt    time.Time
	CurrentTaskID *string
}

// AgentSessionRepository defines the persistence contract for AgentSession.
type AgentSessionRepository interface {
	// Upsert inserts or updates an agent session, refreshing LastSeenAt.
	Upsert(ctx context.Context, s AgentSession) error
	// FindByID retrieves an AgentSession by its agent ID.
	FindByID(ctx context.Context, agentID string) (AgentSession, error)
	// ListActive returns all sessions whose LastSeenAt is within the given
	// duration from now.
	ListActive(ctx context.Context, since time.Duration) ([]AgentSession, error)
	// ClearTask removes the CurrentTaskID from the given agent's session.
	ClearTask(ctx context.Context, agentID string) error
}
