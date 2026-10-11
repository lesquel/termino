package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
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

// seedProject inserts a minimal project row so FK constraints pass.
func seedProject(t *testing.T, db *sql.DB, id, key string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO projects (id, key, name, methodology, created_at)
		VALUES (?, ?, ?, 'kanban', ?)`,
		id, key, key+" Project", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seedProject: %v", err)
	}
}

func TestBoardRepository_SaveAndFindColumn(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-board", "BRD")
	repo := boardinfra.NewBoardRepository(db)
	ctx := context.Background()

	wip := 3
	col := boarddomain.Column{
		ID:        "col-1",
		ProjectID: "proj-board",
		Name:      "In Progress",
		Position:  1,
		WIPLimit:  &wip,
	}
	if err := repo.SaveColumn(ctx, col); err != nil {
		t.Fatalf("SaveColumn: %v", err)
	}

	got, err := repo.FindColumnByID(ctx, "col-1")
	if err != nil {
		t.Fatalf("FindColumnByID: %v", err)
	}
	if got.Name != col.Name {
		t.Errorf("Name: got %q, want %q", got.Name, col.Name)
	}
	if got.WIPLimit == nil || *got.WIPLimit != wip {
		t.Errorf("WIPLimit: got %v, want %d", got.WIPLimit, wip)
	}
}

func TestBoardRepository_ListColumns(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-list", "LST")
	repo := boardinfra.NewBoardRepository(db)
	ctx := context.Background()

	cols := []boarddomain.Column{
		{ID: "c1", ProjectID: "proj-list", Name: "Backlog", Position: 0},
		{ID: "c2", ProjectID: "proj-list", Name: "In Progress", Position: 1},
		{ID: "c3", ProjectID: "proj-list", Name: "Done", Position: 2},
	}
	for _, c := range cols {
		if err := repo.SaveColumn(ctx, c); err != nil {
			t.Fatalf("SaveColumn %s: %v", c.ID, err)
		}
	}

	list, err := repo.ListColumns(ctx, "proj-list")
	if err != nil {
		t.Fatalf("ListColumns: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListColumns: got %d, want 3", len(list))
	}
	if list[0].Name != "Backlog" {
		t.Errorf("first column: got %q, want Backlog", list[0].Name)
	}
}

func TestBoardRepository_ReorderColumns(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-reorder", "RRD")
	repo := boardinfra.NewBoardRepository(db)
	ctx := context.Background()

	for _, c := range []boarddomain.Column{
		{ID: "r1", ProjectID: "proj-reorder", Name: "A", Position: 0},
		{ID: "r2", ProjectID: "proj-reorder", Name: "B", Position: 1},
		{ID: "r3", ProjectID: "proj-reorder", Name: "C", Position: 2},
	} {
		if err := repo.SaveColumn(ctx, c); err != nil {
			t.Fatalf("SaveColumn: %v", err)
		}
	}

	// Reverse the order.
	if err := repo.ReorderColumns(ctx, "proj-reorder", []string{"r3", "r2", "r1"}); err != nil {
		t.Fatalf("ReorderColumns: %v", err)
	}

	list, err := repo.ListColumns(ctx, "proj-reorder")
	if err != nil {
		t.Fatalf("ListColumns: %v", err)
	}
	if list[0].ID != "r3" {
		t.Errorf("first after reorder: got %q, want r3", list[0].ID)
	}
}

func TestBoardRepository_DeleteColumn(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-del", "DEL")
	repo := boardinfra.NewBoardRepository(db)
	ctx := context.Background()

	col := boarddomain.Column{ID: "del-col", ProjectID: "proj-del", Name: "Temp", Position: 0}
	if err := repo.SaveColumn(ctx, col); err != nil {
		t.Fatalf("SaveColumn: %v", err)
	}
	if err := repo.DeleteColumn(ctx, "del-col"); err != nil {
		t.Fatalf("DeleteColumn: %v", err)
	}

	_, err := repo.FindColumnByID(ctx, "del-col")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestBoardRepository_WIPLimitZeroMeansNil(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-wip", "WIP")
	repo := boardinfra.NewBoardRepository(db)
	ctx := context.Background()

	col := boarddomain.Column{
		ID:        "wip-col",
		ProjectID: "proj-wip",
		Name:      "No Limit",
		Position:  0,
		WIPLimit:  nil,
	}
	if err := repo.SaveColumn(ctx, col); err != nil {
		t.Fatalf("SaveColumn: %v", err)
	}
	got, err := repo.FindColumnByID(ctx, "wip-col")
	if err != nil {
		t.Fatalf("FindColumnByID: %v", err)
	}
	if got.WIPLimit != nil {
		t.Errorf("WIPLimit: got %v, want nil", got.WIPLimit)
	}
}
