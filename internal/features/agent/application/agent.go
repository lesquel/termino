// Package application contains the agent feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"fmt"
	"time"

	agentdomain "github.com/lesquel/termino/internal/features/agent/domain"
)

// AgentUseCases groups all agent-related use cases.
type AgentUseCases struct {
	sessions agentdomain.AgentSessionRepository
}

// NewAgentUseCases constructs an AgentUseCases instance.
func NewAgentUseCases(sessions agentdomain.AgentSessionRepository) *AgentUseCases {
	return &AgentUseCases{sessions: sessions}
}

// UpdatePresence upserts the agent session, refreshing LastSeenAt to now.
func (uc *AgentUseCases) UpdatePresence(
	ctx context.Context,
	session agentdomain.AgentSession,
) error {
	session.LastSeenAt = time.Now().UTC()
	if err := uc.sessions.Upsert(ctx, session); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	return nil
}

// ListActiveAgents returns all agent sessions last seen within the given duration.
func (uc *AgentUseCases) ListActiveAgents(
	ctx context.Context,
	since time.Duration,
) ([]agentdomain.AgentSession, error) {
	return uc.sessions.ListActive(ctx, since)
}

// ClaimTask assigns a task to an agent and updates the agent's current task.
// The task-level move use case should be invoked by the caller; this use case
// only updates the presence record.
func (uc *AgentUseCases) ClaimTask(
	ctx context.Context,
	agentID, taskID string,
) error {
	s, err := uc.sessions.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("find agent session: %w", err)
	}
	s.CurrentTaskID = &taskID
	s.LastSeenAt = time.Now().UTC()
	return uc.sessions.Upsert(ctx, s)
}
