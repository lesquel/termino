package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
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

func TestMemberRepository_SaveAndFind(t *testing.T) {
	db := openTestDB(t)
	repo := memberinfra.NewMemberRepository(db)
	ctx := context.Background()

	m := memberdomain.Member{
		ID:   "mem-1",
		Name: "Alice",
		Kind: memberdomain.KindHuman,
	}
	if err := repo.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, "mem-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Name != m.Name {
		t.Errorf("Name: got %q, want %q", got.Name, m.Name)
	}
	if got.Kind != memberdomain.KindHuman {
		t.Errorf("Kind: got %q, want human", got.Kind)
	}
}

func TestMemberRepository_FindByName(t *testing.T) {
	db := openTestDB(t)
	repo := memberinfra.NewMemberRepository(db)
	ctx := context.Background()

	m := memberdomain.Member{ID: "mem-2", Name: "Bob", Kind: memberdomain.KindAgent}
	if err := repo.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByName(ctx, "Bob")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if got.ID != "mem-2" {
		t.Errorf("ID: got %q, want mem-2", got.ID)
	}
}

func TestMemberRepository_List(t *testing.T) {
	db := openTestDB(t)
	repo := memberinfra.NewMemberRepository(db)
	ctx := context.Background()

	// Count existing members (system members inserted by migration).
	existing, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List (baseline): %v", err)
	}
	baseline := len(existing)

	for i, name := range []string{"Charlie", "Dana"} {
		_ = i
		m := memberdomain.Member{ID: "mem-" + name, Name: name, Kind: memberdomain.KindHuman}
		if err := repo.Save(ctx, m); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != baseline+2 {
		t.Errorf("List: got %d, want %d", len(list), baseline+2)
	}
}

func TestMemberRepository_Delete(t *testing.T) {
	db := openTestDB(t)
	repo := memberinfra.NewMemberRepository(db)
	ctx := context.Background()

	m := memberdomain.Member{ID: "mem-del", Name: "Eve", Kind: memberdomain.KindHuman}
	if err := repo.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, "mem-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.FindByID(ctx, "mem-del")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
