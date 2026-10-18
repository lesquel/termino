package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lesquel/termino/internal/features/share/domain"
)

// ImportStateRepository stores when each shared project was last imported
// and which files that import skipped (share_imports, share_skipped). Every
// termino process on the machine reads the same state, so `termino share
// status` and the board warning see what the TUI or MCP watcher found.
type ImportStateRepository struct {
	db *sql.DB
}

// NewImportStateRepository constructs an ImportStateRepository backed by db.
func NewImportStateRepository(db *sql.DB) *ImportStateRepository {
	return &ImportStateRepository{db: db}
}

// RecordImport replaces the import state of the project.
func (r *ImportStateRepository) RecordImport(ctx context.Context, projectID string, at time.Time, skipped []domain.Skipped) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("share_imports begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO share_imports (project_id, imported_at) VALUES (?, ?)
		ON CONFLICT(project_id) DO UPDATE SET imported_at = excluded.imported_at`,
		projectID, at.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("share_imports.Put: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM share_skipped WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("share_skipped.Clear: %w", err)
	}
	for _, s := range skipped {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO share_skipped (project_id, path, reason) VALUES (?, ?, ?)`,
			projectID, s.Path, s.Reason); err != nil {
			return fmt.Errorf("share_skipped.Put: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("share_imports commit: %w", err)
	}
	return nil
}

// LastImport returns the state recorded by the last import of the project;
// a project never imported yields a zero ImportStatus.
func (r *ImportStateRepository) LastImport(ctx context.Context, projectID string) (domain.ImportStatus, error) {
	var st domain.ImportStatus
	var at string
	err := r.db.QueryRowContext(ctx,
		`SELECT imported_at FROM share_imports WHERE project_id = ?`, projectID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("share_imports.Get: %w", err)
	}
	st.At, _ = time.Parse(time.RFC3339, at)
	rows, err := r.db.QueryContext(ctx,
		`SELECT path, reason FROM share_skipped WHERE project_id = ? ORDER BY path`, projectID)
	if err != nil {
		return st, fmt.Errorf("share_skipped.List: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s domain.Skipped
		if err := rows.Scan(&s.Path, &s.Reason); err != nil {
			return st, fmt.Errorf("share_skipped scan: %w", err)
		}
		st.Skipped = append(st.Skipped, s)
	}
	return st, rows.Err()
}

// SetOptOut records (on) or forgets (off) that the project stopped sharing
// from root.
func (r *ImportStateRepository) SetOptOut(ctx context.Context, projectID, root string, on bool) error {
	q := `DELETE FROM share_optouts WHERE project_id = ? AND root = ?`
	if on {
		q = `INSERT OR IGNORE INTO share_optouts (project_id, root) VALUES (?, ?)`
	}
	if _, err := r.db.ExecContext(ctx, q, projectID, root); err != nil {
		return fmt.Errorf("share_optouts: %w", err)
	}
	return nil
}

// OptedOut reports whether the project stopped sharing from root.
func (r *ImportStateRepository) OptedOut(ctx context.Context, projectID, root string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM share_optouts WHERE project_id = ? AND root = ?`, projectID, root).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("share_optouts: %w", err)
	}
	return n > 0, nil
}
