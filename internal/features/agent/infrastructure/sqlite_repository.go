// Package infrastructure provides SQLite-backed implementations of the agent
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/lesquel/termino/internal/features/agent/domain"
)

// AgentSessionRepository is the SQLite-backed implementation of
// agentdomain.AgentSessionRepository.
type AgentSessionRepository struct {
	db *sql.DB
}

// NewAgentSessionRepository constructs an AgentSessionRepository backed by db.
func NewAgentSessionRepository(db *sql.DB) *AgentSessionRepository {
	return &AgentSessionRepository{db: db}
}

// Upsert inserts or replaces an agent session using INSERT OR REPLACE on agent_id.
func (r *AgentSessionRepository) Upsert(ctx context.Context, s agentdomain.AgentSession) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO agent_sessions (agent_id, agent_name, last_seen_at, current_task_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			agent_name      = excluded.agent_name,
			last_seen_at    = excluded.last_seen_at,
			current_task_id = excluded.current_task_id`,
		s.AgentID,
		s.AgentName,
		s.LastSeenAt.UTC().Format(time.RFC3339),
		s.CurrentTaskID,
	)
	if err != nil {
		return fmt.Errorf("agentSession.Upsert: %w", err)
	}
	return nil
}

// FindByID retrieves an AgentSession by its agent ID.
func (r *AgentSessionRepository) FindByID(ctx context.Context, agentID string) (agentdomain.AgentSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT agent_id, agent_name, last_seen_at, current_task_id
		FROM   agent_sessions
		WHERE  agent_id = ?`, agentID)
	return scanAgentSession(row)
}

// ListActive returns sessions whose last_seen_at is within since from now.
func (r *AgentSessionRepository) ListActive(ctx context.Context, since time.Duration) ([]agentdomain.AgentSession, error) {
	cutoff := time.Now().UTC().Add(-since).Format(time.RFC3339)
	rows, err := r.db.QueryContext(ctx, `
		SELECT agent_id, agent_name, last_seen_at, current_task_id
		FROM   agent_sessions
		WHERE  last_seen_at > ?
		ORDER  BY last_seen_at DESC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("agentSession.ListActive: %w", err)
	}
	defer rows.Close()

	var sessions []agentdomain.AgentSession
	for rows.Next() {
		s, err := scanAgentSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// ClearTask removes the CurrentTaskID from the given agent's session.
func (r *AgentSessionRepository) ClearTask(ctx context.Context, agentID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE agent_sessions SET current_task_id = NULL WHERE agent_id = ?`, agentID)
	if err != nil {
		return fmt.Errorf("agentSession.ClearTask: %w", err)
	}
	return nil
}

// scanAgentSession scans a single agent session row from a *sql.Row.
func scanAgentSession(row *sql.Row) (agentdomain.AgentSession, error) {
	var s agentdomain.AgentSession
	var lastSeenAt string
	err := row.Scan(&s.AgentID, &s.AgentName, &lastSeenAt, &s.CurrentTaskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentdomain.AgentSession{}, fmt.Errorf("agent session not found")
		}
		return agentdomain.AgentSession{}, fmt.Errorf("agentSession scan: %w", err)
	}
	s.LastSeenAt, err = time.Parse(time.RFC3339, lastSeenAt)
	if err != nil {
		return agentdomain.AgentSession{}, fmt.Errorf("agentSession parse last_seen_at: %w", err)
	}
	return s, nil
}

// scanAgentSessionRow scans a single agent session row from *sql.Rows.
func scanAgentSessionRow(rows *sql.Rows) (agentdomain.AgentSession, error) {
	var s agentdomain.AgentSession
	var lastSeenAt string
	if err := rows.Scan(&s.AgentID, &s.AgentName, &lastSeenAt, &s.CurrentTaskID); err != nil {
		return agentdomain.AgentSession{}, fmt.Errorf("agentSession row scan: %w", err)
	}
	var err error
	s.LastSeenAt, err = time.Parse(time.RFC3339, lastSeenAt)
	if err != nil {
		return agentdomain.AgentSession{}, fmt.Errorf("agentSession parse last_seen_at: %w", err)
	}
	return s, nil
}
