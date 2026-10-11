// Package infrastructure provides SQLite-backed implementations of the member
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
)

// MemberRepository is the SQLite-backed implementation of
// memberdomain.MemberRepository.
type MemberRepository struct {
	db *sql.DB
}

// NewMemberRepository constructs a MemberRepository backed by db.
func NewMemberRepository(db *sql.DB) *MemberRepository {
	return &MemberRepository{db: db}
}

// Save inserts or updates a Member row.
func (r *MemberRepository) Save(ctx context.Context, m memberdomain.Member) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO members (id, name, kind, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind`,
		m.ID, m.Name, string(m.Kind),
		m.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("member.Save: %w", err)
	}
	return nil
}

// FindByID retrieves a Member by its UUID.
func (r *MemberRepository) FindByID(ctx context.Context, id string) (memberdomain.Member, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, kind, created_at
		FROM   members
		WHERE  id = ?`, id)
	return scanMember(row)
}

// FindByName retrieves a Member by name (case-sensitive).
func (r *MemberRepository) FindByName(ctx context.Context, name string) (memberdomain.Member, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, kind, created_at
		FROM   members
		WHERE  name = ?`, name)
	return scanMember(row)
}

// List returns all members in the workspace.
func (r *MemberRepository) List(ctx context.Context) ([]memberdomain.Member, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, kind, created_at
		FROM   members
		ORDER  BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("member.List: %w", err)
	}
	defer rows.Close()

	var members []memberdomain.Member
	for rows.Next() {
		m, err := scanMemberRow(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// Delete permanently removes a Member by ID.
func (r *MemberRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM members WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("member.Delete: %w", err)
	}
	return nil
}

// parseTime parses a time string that may be in RFC3339 or SQLite's native
// datetime format ("2006-01-02 15:04:05" or "2006-01-02 15:04:05.999999999").
func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time format: %q", s)
}

// scanMember scans a single member row from a *sql.Row.
func scanMember(row *sql.Row) (memberdomain.Member, error) {
	var m memberdomain.Member
	var kind, createdAt string
	err := row.Scan(&m.ID, &m.Name, &kind, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return memberdomain.Member{}, fmt.Errorf("member not found")
		}
		return memberdomain.Member{}, fmt.Errorf("member scan: %w", err)
	}
	m.Kind = memberdomain.Kind(kind)
	m.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return memberdomain.Member{}, fmt.Errorf("member parse created_at: %w", err)
	}
	return m, nil
}

// scanMemberRow scans a single member row from *sql.Rows.
func scanMemberRow(rows *sql.Rows) (memberdomain.Member, error) {
	var m memberdomain.Member
	var kind, createdAt string
	if err := rows.Scan(&m.ID, &m.Name, &kind, &createdAt); err != nil {
		return memberdomain.Member{}, fmt.Errorf("member row scan: %w", err)
	}
	m.Kind = memberdomain.Kind(kind)
	var err error
	m.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return memberdomain.Member{}, fmt.Errorf("member parse created_at: %w", err)
	}
	return m, nil
}
