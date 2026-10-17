package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	sprintinfra "github.com/lesquel/termino/internal/features/sprint/infrastructure"
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

func TestSprintRepository_SaveAndFindByID(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-sp", "SPR")
	repo := sprintinfra.NewSprintRepository(db)
	ctx := context.Background()

	s := sprintdomain.Sprint{
		ID:        "sprint-1",
		ProjectID: "proj-sp",
		Name:      "Sprint 1",
		StartsAt:  time.Now().UTC().Truncate(time.Second),
		EndsAt:    time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Second),
		State:     sprintdomain.SprintStateDraft,
	}
	if err := repo.Save(ctx, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, "sprint-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Name != s.Name {
		t.Errorf("Name: got %q, want %q", got.Name, s.Name)
	}
	if got.State != sprintdomain.SprintStateDraft {
		t.Errorf("State: got %q, want draft", got.State)
	}
}

func TestSprintRepository_FindActive(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-act-sp", "ASP")
	repo := sprintinfra.NewSprintRepository(db)
	ctx := context.Background()

	// No active sprint yet.
	active, err := repo.FindActive(ctx, "proj-act-sp")
	if err != nil {
		t.Fatalf("FindActive (none): %v", err)
	}
	if active != nil {
		t.Error("expected nil active sprint, got one")
	}

	s := sprintdomain.Sprint{
		ID:        "sprint-active",
		ProjectID: "proj-act-sp",
		Name:      "Active Sprint",
		StartsAt:  time.Now().UTC().Truncate(time.Second),
		EndsAt:    time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Second),
		State:     sprintdomain.SprintStateActive,
	}
	if err := repo.Save(ctx, s); err != nil {
		t.Fatalf("Save active: %v", err)
	}

	got, err := repo.FindActive(ctx, "proj-act-sp")
	if err != nil {
		t.Fatalf("FindActive: %v", err)
	}
	if got == nil {
		t.Fatal("expected active sprint, got nil")
	}
	if got.ID != "sprint-active" {
		t.Errorf("ID: got %q, want sprint-active", got.ID)
	}
}

func TestSprintRepository_ListByProject(t *testing.T) {
	db := openTestDB(t)
	seedProject(t, db, "proj-list-sp", "LSP")
	repo := sprintinfra.NewSprintRepository(db)
	ctx := context.Background()

	for _, name := range []string{"Sprint 1", "Sprint 2"} {
		s := sprintdomain.Sprint{
			ID:        "sp-" + name,
			ProjectID: "proj-list-sp",
			Name:      name,
			StartsAt:  time.Now().UTC().Truncate(time.Second),
			EndsAt:    time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Second),
			State:     sprintdomain.SprintStateDraft,
		}
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	list, err := repo.ListByProject(ctx, "proj-list-sp")
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListByProject: got %d, want 2", len(list))
	}
}
