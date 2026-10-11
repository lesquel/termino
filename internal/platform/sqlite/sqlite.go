// Package sqlite provides database connection management and automatic
// SQL migrations for Termino's SQLite data store.
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; registers "sqlite" driver name
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationsDir is the subdirectory prefix used inside the embedded FS.
const migrationsDir = "migrations"

// Open opens (or creates) the SQLite database at path, applies all pending
// migrations, and configures the connection for concurrent use.
// The containing directory is created with 0700; the database file gets 0600.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("sqlite.Open: mkdir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite.Open: open: %w", err)
	}

	// Single writer: prevents SQLITE_BUSY from concurrent writes in the same
	// process (WAL handles multi-process concurrency).
	db.SetMaxOpenConns(1)

	if err := applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		// Best-effort: newly created files may not exist until first write.
		_ = err
	}

	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite.Open: migrations: %w", err)
	}

	return db, nil
}

// OpenReadOnly opens an existing database at path in read-only mode.
// Used by the widget command to avoid locking the DB for writers.
func OpenReadOnly(path string) (*sql.DB, error) {
	// The "?mode=ro" URI parameter asks SQLite to open read-only.
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("sqlite.OpenReadOnly: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite.OpenReadOnly: pragma: %w", err)
	}
	return db, nil
}

// applyPragmas sets the recommended SQLite PRAGMAs for Termino.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("sqlite: pragma %q: %w", p, err)
		}
	}
	return nil
}

// runMigrations ensures the schema_migrations table exists and applies any
// migration files that have not yet been recorded there.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name       TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	files, err := collectMigrationFiles()
	if err != nil {
		return err
	}

	for _, name := range files {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists > 0 {
			continue
		}

		content, err := migrationFiles.ReadFile(migrationsDir + "/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations(name) VALUES(?)`, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}

// collectMigrationFiles returns embedded *.sql filenames sorted lexicographically,
// which preserves the NNN_ numeric prefix ordering.
func collectMigrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationFiles, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
