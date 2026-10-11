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

// DetectForPath returns the project ID whose associated_path is the longest
// prefix of fsPath. Returns an empty string (no error) when no match exists.
func (d *PathDetector) DetectForPath(fsPath string) (string, error) {
	// Use parameterized LIKE to find rows where associated_path is a prefix.
	// We append '%' to the path as the wildcard; the rows are ordered longest
	// first so that the most-specific path wins.
	row := d.db.QueryRow(`
		SELECT id
		FROM   projects
		WHERE  archived_at IS NULL
		  AND  associated_path IS NOT NULL
		  AND  ? LIKE (associated_path || '%')
		ORDER  BY length(associated_path) DESC
		LIMIT  1`, fsPath)

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
