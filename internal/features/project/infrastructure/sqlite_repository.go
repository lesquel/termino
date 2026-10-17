// Package infrastructure provides SQLite-backed implementations of the project
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
)

// ProjectRepository is the SQLite-backed implementation of
// projectdomain.ProjectRepository.
type ProjectRepository struct {
	db *sql.DB
}

// NewProjectRepository constructs a ProjectRepository backed by db.
func NewProjectRepository(db *sql.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// Save inserts or updates a Project row.
func (r *ProjectRepository) Save(ctx context.Context, p projectdomain.Project) error {
	var archivedAt *string
	if p.ArchivedAt != nil {
		s := p.ArchivedAt.UTC().Format(time.RFC3339)
		archivedAt = &s
	}
	var associatedPath *string
	if p.AssociatedPath != "" {
		associatedPath = &p.AssociatedPath
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO projects (id, key, name, methodology, created_at, archived_at, associated_path, share_root)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			key             = excluded.key,
			name            = excluded.name,
			methodology     = excluded.methodology,
			archived_at     = excluded.archived_at,
			associated_path = excluded.associated_path,
			share_root      = excluded.share_root`,
		p.ID,
		p.Key,
		p.Name,
		string(p.Methodology),
		p.CreatedAt.UTC().Format(time.RFC3339),
		archivedAt,
		associatedPath,
		p.ShareRoot,
	)
	if err != nil {
		return fmt.Errorf("project.Save: %w", err)
	}
	return nil
}

// FindByID retrieves a non-archived project by its UUID.
func (r *ProjectRepository) FindByID(ctx context.Context, id string) (projectdomain.Project, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, key, name, methodology, created_at, archived_at, associated_path, share_root
		FROM   projects
		WHERE  id = ? AND archived_at IS NULL`, id)
	return scanProject(row)
}

// FindByKey retrieves a non-archived project by its short key.
func (r *ProjectRepository) FindByKey(ctx context.Context, key string) (projectdomain.Project, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, key, name, methodology, created_at, archived_at, associated_path, share_root
		FROM   projects
		WHERE  key = ? AND archived_at IS NULL`, key)
	return scanProject(row)
}

// List returns all non-archived projects ordered by created_at ascending.
func (r *ProjectRepository) List(ctx context.Context) ([]projectdomain.Project, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, key, name, methodology, created_at, archived_at, associated_path, share_root
		FROM   projects
		WHERE  archived_at IS NULL
		ORDER  BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("project.List: %w", err)
	}
	defer rows.Close()
	return collectProjects(rows)
}

// Archive sets the archived_at timestamp on a project.
func (r *ProjectRepository) Archive(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE projects SET archived_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("project.Archive: %w", err)
	}
	return nil
}

// scanProject scans a single project row from a *sql.Row.
func scanProject(row *sql.Row) (projectdomain.Project, error) {
	var p projectdomain.Project
	var createdAt string
	var archivedAt *string
	var associatedPath sql.NullString
	err := row.Scan(&p.ID, &p.Key, &p.Name, &p.Methodology, &createdAt, &archivedAt, &associatedPath, &p.ShareRoot)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return projectdomain.Project{}, fmt.Errorf("project not found")
		}
		return projectdomain.Project{}, fmt.Errorf("project scan: %w", err)
	}
	p.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return projectdomain.Project{}, fmt.Errorf("project parse created_at: %w", err)
	}
	if archivedAt != nil {
		t, err := time.Parse(time.RFC3339, *archivedAt)
		if err != nil {
			return projectdomain.Project{}, fmt.Errorf("project parse archived_at: %w", err)
		}
		p.ArchivedAt = &t
	}
	p.AssociatedPath = associatedPath.String
	return p, nil
}

// collectProjects reads all rows from a *sql.Rows into a slice of Project.
func collectProjects(rows *sql.Rows) ([]projectdomain.Project, error) {
	var projects []projectdomain.Project
	for rows.Next() {
		var p projectdomain.Project
		var createdAt string
		var archivedAt *string
		var associatedPath sql.NullString
		if err := rows.Scan(&p.ID, &p.Key, &p.Name, &p.Methodology, &createdAt, &archivedAt, &associatedPath, &p.ShareRoot); err != nil {
			return nil, fmt.Errorf("project row scan: %w", err)
		}
		var err error
		p.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("project parse created_at: %w", err)
		}
		if archivedAt != nil {
			t, err := time.Parse(time.RFC3339, *archivedAt)
			if err != nil {
				return nil, fmt.Errorf("project parse archived_at: %w", err)
			}
			p.ArchivedAt = &t
		}
		p.AssociatedPath = associatedPath.String
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
