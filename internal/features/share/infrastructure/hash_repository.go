package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// HashRepository is the SQLite store of share_files: the content hash of
// every shared file last written or read, per project and path relative to
// the .termino directory. It is shared by every termino process on the
// machine, so none of them re-imports another one's writes.
type HashRepository struct {
	db *sql.DB
}

// NewHashRepository constructs a HashRepository backed by db.
func NewHashRepository(db *sql.DB) *HashRepository {
	return &HashRepository{db: db}
}

// Get returns the stored hash for path; ok is false when none is stored.
func (r *HashRepository) Get(ctx context.Context, projectID, path string) (hash string, ok bool, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT hash FROM share_files WHERE project_id = ? AND path = ?`, projectID, path).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("share_files.Get: %w", err)
	}
	return hash, true, nil
}

// Put stores hash for path, replacing any previous value.
func (r *HashRepository) Put(ctx context.Context, projectID, path, hash string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO share_files (project_id, path, hash) VALUES (?, ?, ?)
		ON CONFLICT(project_id, path) DO UPDATE SET hash = excluded.hash`,
		projectID, path, hash)
	if err != nil {
		return fmt.Errorf("share_files.Put: %w", err)
	}
	return nil
}

// Delete forgets the hash stored for path.
func (r *HashRepository) Delete(ctx context.Context, projectID, path string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM share_files WHERE project_id = ? AND path = ?`, projectID, path)
	if err != nil {
		return fmt.Errorf("share_files.Delete: %w", err)
	}
	return nil
}

// List returns every stored hash of the project, keyed by path.
func (r *HashRepository) List(ctx context.Context, projectID string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT path, hash FROM share_files WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("share_files.List: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, fmt.Errorf("share_files.List scan: %w", err)
		}
		out[path] = hash
	}
	return out, rows.Err()
}
