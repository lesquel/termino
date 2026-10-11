package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	widgetinfra "github.com/lesquel/termino/internal/features/widget/infrastructure"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestWidgetRepository_GetSummaryEmptyProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Seed a project and a column (required for widget queries).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO projects (id, key, name, methodology, created_at)
		VALUES ('wp1', 'WDG', 'Widget Project', 'kanban', ?)`, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO columns (id, project_id, name, position, wip_limit)
		VALUES ('wc1', 'wp1', 'In Progress', 1, 0)`); err != nil {
		t.Fatalf("seed column: %v", err)
	}

	repo := widgetinfra.NewWidgetRepository(db)
	summary, err := repo.GetSummary(ctx, "wp1", "cli")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.ProjectName != "Widget Project" {
		t.Errorf("ProjectName: got %q, want Widget Project", summary.ProjectName)
	}
	if summary.ProjectKey != "WDG" {
		t.Errorf("ProjectKey: got %q, want WDG", summary.ProjectKey)
	}
	// No tasks seeded — all slices should be empty.
	if len(summary.AssignedTasks) != 0 {
		t.Errorf("AssignedTasks: got %d, want 0", len(summary.AssignedTasks))
	}
}
