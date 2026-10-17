package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
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

func TestProjectRepository_SaveAndFind(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	ctx := context.Background()

	p := projectdomain.Project{
		ID:          "proj-1",
		Key:         "WEB",
		Name:        "Web App",
		Methodology: projectdomain.MethodologyKanban,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, "proj-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Key != p.Key {
		t.Errorf("Key: got %q, want %q", got.Key, p.Key)
	}
	if got.Name != p.Name {
		t.Errorf("Name: got %q, want %q", got.Name, p.Name)
	}
}

func TestProjectRepository_FindByKey(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	ctx := context.Background()

	p := projectdomain.Project{
		ID:          "proj-2",
		Key:         "API",
		Name:        "API Service",
		Methodology: projectdomain.MethodologyScrum,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByKey(ctx, "API")
	if err != nil {
		t.Fatalf("FindByKey: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("ID: got %q, want %q", got.ID, p.ID)
	}
}

func TestProjectRepository_List(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	ctx := context.Background()

	for i, key := range []string{"PRJ", "XYZ"} {
		_ = i
		p := projectdomain.Project{
			ID:          "list-" + key,
			Key:         key,
			Name:        key + " Project",
			Methodology: projectdomain.MethodologyKanban,
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		if err := repo.Save(ctx, p); err != nil {
			t.Fatalf("Save %s: %v", key, err)
		}
	}

	projects, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("List: got %d projects, want 2", len(projects))
	}
}

func TestProjectRepository_Archive(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	ctx := context.Background()

	p := projectdomain.Project{
		ID:          "proj-arch",
		Key:         "ARC",
		Name:        "Archive Me",
		Methodology: projectdomain.MethodologyKanban,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	at := time.Now().UTC().Truncate(time.Second)
	if err := repo.Archive(ctx, "proj-arch", at); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// FindByID on an archived project should return not-found.
	_, err := repo.FindByID(ctx, "proj-arch")
	if err == nil {
		t.Error("expected error for archived project, got nil")
	}

	// List should not include the archived project.
	projects, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, proj := range projects {
		if proj.ID == "proj-arch" {
			t.Error("archived project appeared in List()")
		}
	}
}

func TestProjectRepository_DuplicateKey(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	ctx := context.Background()

	p1 := projectdomain.Project{
		ID:          "dup-1",
		Key:         "DUP",
		Name:        "Duplicate 1",
		Methodology: projectdomain.MethodologyKanban,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Save(ctx, p1); err != nil {
		t.Fatalf("Save first: %v", err)
	}

	p2 := projectdomain.Project{
		ID:          "dup-2",
		Key:         "DUP",
		Name:        "Duplicate 2",
		Methodology: projectdomain.MethodologyKanban,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	err := repo.Save(ctx, p2)
	if err == nil {
		t.Error("expected unique constraint error for duplicate key, got nil")
	}
}

func TestPathDetector_DetectForPath(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	detector := projectinfra.NewPathDetector(db)
	ctx := context.Background()

	p := projectdomain.Project{
		ID:          "path-proj",
		Key:         "PTH",
		Name:        "Path Project",
		Methodology: projectdomain.MethodologyKanban,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Set associated_path directly.
	if _, err := db.Exec(`UPDATE projects SET associated_path = ? WHERE id = ?`,
		"/home/user/myproject", "path-proj"); err != nil {
		t.Fatalf("set associated_path: %v", err)
	}

	id, err := detector.DetectForPath("/home/user/myproject/src/main.go")
	if err != nil {
		t.Fatalf("DetectForPath: %v", err)
	}
	if id != "path-proj" {
		t.Errorf("DetectForPath: got %q, want %q", id, "path-proj")
	}

	// No match for an unrelated path.
	id, err = detector.DetectForPath("/tmp/other")
	if err != nil {
		t.Fatalf("DetectForPath no match: %v", err)
	}
	if id != "" {
		t.Errorf("DetectForPath no match: got %q, want empty", id)
	}
}
