package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lesquel/termino/internal/platform/sqlite"
)

func TestOpen_WALMode(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q; want \"wal\"", mode)
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys = %d; want 1", enabled)
	}
}

func TestOpen_MigrationsRun(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	// The projects table must exist after migrations.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		t.Fatalf("projects table not found after migrations: %v", err)
	}

	// At least 2 migration files should be recorded.
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("schema_migrations table: %v", err)
	}
	if n < 2 {
		t.Errorf("schema_migrations count = %d; want >= 2", n)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("first Open() error: %v", err)
	}
	db1.Close()

	db2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("second Open() error: %v", err)
	}
	db2.Close()
}

func TestOpen_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.db")

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	// Force SQLite to flush/create the file by doing a write.
	if _, err := db.Exec(`INSERT OR IGNORE INTO members(id,name,kind) VALUES('test-id','tester','human')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	db.Close()

	// The containing directory must be 0700.
	info, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatalf("Stat sub dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("data dir perm = %04o; want 0700", perm)
	}

	// The database file must be 0600.
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("Stat db file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("db file perm = %04o; want 0600", perm)
	}
}

func TestOpen_ShareMigration(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer db.Close()

	for _, q := range []string{
		`SELECT share_root FROM projects`,
		`SELECT updated_by FROM tasks`,
		`SELECT project_id, path, hash FROM share_files`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Errorf("%s: %v", q, err)
			continue
		}
		_ = rows.Close()
	}
}
