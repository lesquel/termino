// Package infrastructure provides SQLite-backed implementations of the project
// domain repository interfaces.
package infrastructure

import (
	"database/sql"
	"errors"
	"fmt"
)

// PathDetector implements projectdomain.ProjectDetector by querying the
// projects table for the row whose associated_path is the longest prefix of
// the given filesystem path.
type PathDetector struct {
	db *sql.DB
}

// NewPathDetector constructs a PathDetector backed by db.
func NewPathDetector(db *sql.DB) *PathDetector {
	return &PathDetector{db: db}
}

// DetectForPath returns the project ID whose associated_path or share_root
// is the longest path-segment prefix of fsPath. Returns an empty string (no error) when no
// match exists.
//
// The match is a true path-prefix, not a raw string prefix: "/home/a" only
// matches fsPath "/home/a" or a descendant such as "/home/a/sub", never
// "/home/abc". The stored associated_path is normalised by stripping trailing
// slashes so a path stored with a trailing slash still matches its
// descendants.
func (d *PathDetector) DetectForPath(fsPath string) (string, error) {
	// Bind fsPath twice: once for the exact match, once for the descendant
	// match. Rows are ordered longest first so the most-specific path wins.
	// A shared project also matches every directory inside its share root
	// (the git root holding .termino/), so a clone picks the shared board; on
	// an equally long match the share root wins.
	row := d.db.QueryRow(`
		SELECT id FROM (
			SELECT id, rtrim(associated_path, '/') AS root, 1 AS rank
			FROM   projects
			WHERE  archived_at IS NULL AND associated_path IS NOT NULL AND associated_path <> ''
			UNION ALL
			SELECT id, rtrim(share_root, '/') AS root, 0 AS rank
			FROM   projects
			WHERE  archived_at IS NULL AND share_root <> ''
		)
		WHERE  ? = root OR ? LIKE (root || '/%')
		ORDER  BY length(root) DESC, rank ASC
		LIMIT  1`, fsPath, fsPath)

	var id string
	err := row.Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("PathDetector.DetectForPath: %w", err)
	}
	return id, nil
}
