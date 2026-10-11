// Package infrastructure provides SQLite-backed implementations of the sprint
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
)

// SprintRepository is the SQLite-backed implementation of
// sprintdomain.SprintRepository.
type SprintRepository struct {
	db *sql.DB
}

// NewSprintRepository constructs a SprintRepository backed by db.
func NewSprintRepository(db *sql.DB) *SprintRepository {
	return &SprintRepository{db: db}
}

// Save inserts or updates a Sprint row.
func (r *SprintRepository) Save(ctx context.Context, s sprintdomain.Sprint) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sprints (id, project_id, name, starts_at, ends_at, state)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name      = excluded.name,
			starts_at = excluded.starts_at,
			ends_at   = excluded.ends_at,
			state     = excluded.state`,
		s.ID, s.ProjectID, s.Name,
		s.StartsAt.UTC().Format(time.RFC3339),
		s.EndsAt.UTC().Format(time.RFC3339),
		string(s.State),
	)
	if err != nil {
		return fmt.Errorf("sprint.Save: %w", err)
	}
	return nil
}

// FindByID retrieves a Sprint by its UUID.
func (r *SprintRepository) FindByID(ctx context.Context, id string) (sprintdomain.Sprint, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, starts_at, ends_at, state
		FROM   sprints
		WHERE  id = ?`, id)
	return scanSprint(row)
}

// FindActive returns the active sprint for the project, or nil if none exists.
func (r *SprintRepository) FindActive(ctx context.Context, projectID string) (*sprintdomain.Sprint, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, starts_at, ends_at, state
		FROM   sprints
		WHERE  state = 'active' AND project_id = ?
		LIMIT  1`, projectID)
	s, err := scanSprint(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// ListByProject returns all sprints for the given project.
func (r *SprintRepository) ListByProject(ctx context.Context, projectID string) ([]sprintdomain.Sprint, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, project_id, name, starts_at, ends_at, state
		FROM   sprints
		WHERE  project_id = ?
		ORDER  BY starts_at ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("sprint.ListByProject: %w", err)
	}
	defer rows.Close()

	var sprints []sprintdomain.Sprint
	for rows.Next() {
		var s sprintdomain.Sprint
		var startsAt, endsAt, state string
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &startsAt, &endsAt, &state); err != nil {
			return nil, fmt.Errorf("sprint row scan: %w", err)
		}
		s.State = sprintdomain.SprintState(state)
		s.StartsAt, err = time.Parse(time.RFC3339, startsAt)
		if err != nil {
			return nil, fmt.Errorf("sprint parse starts_at: %w", err)
		}
		s.EndsAt, err = time.Parse(time.RFC3339, endsAt)
		if err != nil {
			return nil, fmt.Errorf("sprint parse ends_at: %w", err)
		}
		sprints = append(sprints, s)
	}
	return sprints, rows.Err()
}

// scanSprint scans a single sprint row.
func scanSprint(row *sql.Row) (sprintdomain.Sprint, error) {
	var s sprintdomain.Sprint
	var startsAt, endsAt, state string
	err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &startsAt, &endsAt, &state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sprintdomain.Sprint{}, sql.ErrNoRows
		}
		return sprintdomain.Sprint{}, fmt.Errorf("sprint scan: %w", err)
	}
	s.State = sprintdomain.SprintState(state)
	s.StartsAt, err = time.Parse(time.RFC3339, startsAt)
	if err != nil {
		return sprintdomain.Sprint{}, fmt.Errorf("sprint parse starts_at: %w", err)
	}
	s.EndsAt, err = time.Parse(time.RFC3339, endsAt)
	if err != nil {
		return sprintdomain.Sprint{}, fmt.Errorf("sprint parse ends_at: %w", err)
	}
	return s, nil
}

// isNotFound checks if an error message contains "not found".
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "sprint not found"
}
