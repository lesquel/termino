package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lesquel/termino/internal/features/share/infrastructure"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

func TestFileStore_WriteReadListRemove(t *testing.T) {
	root := t.TempDir()
	fs := infrastructure.FileStore{}
	dir := filepath.Join(root, ".termino", "tasks")
	p := filepath.Join(dir, "a.toml")
	if err := fs.Write(p, []byte("one")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := fs.Write(p, []byte("two")); err != nil {
		t.Fatalf("Write again: %v", err)
	}
	got, err := fs.Read(p)
	if err != nil || string(got) != "two" {
		t.Fatalf("Read = %q, %v", got, err)
	}
	// Temp and hidden files are not listed.
	_ = os.WriteFile(filepath.Join(dir, ".b.toml.tmp"), nil, 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".hidden.toml"), nil, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.toml"), nil, 0o644)
	names, err := fs.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"a.toml", "c.toml"}) {
		t.Errorf("List = %v", names)
	}
	if err := fs.Remove(p); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := fs.Remove(p); err != nil {
		t.Errorf("Remove missing file must not fail: %v", err)
	}
	if _, err := fs.Read(p); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Read removed = %v", err)
	}
	if names, err := fs.List(filepath.Join(root, "missing")); err != nil || len(names) != 0 {
		t.Errorf("List missing dir = %v, %v", names, err)
	}
}

func TestFindGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := infrastructure.FindGitRoot(deep)
	if err != nil || got != root {
		t.Errorf("FindGitRoot = %q, %v; want %q", got, err, root)
	}

	// A worktree has a .git file instead of a directory.
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := infrastructure.FindGitRoot(wt); err != nil || got != wt {
		t.Errorf("FindGitRoot worktree = %q, %v", got, err)
	}
}

func TestFindGitRoot_NotARepo(t *testing.T) {
	if _, err := infrastructure.FindGitRoot(t.TempDir()); !errors.Is(err, infrastructure.ErrNotGitRepository) {
		// /tmp could sit inside a repository on an unusual machine; only the
		// error type is checked when one is returned.
		if err != nil {
			t.Errorf("err = %v, want ErrNotGitRepository", err)
		}
	}
}

func TestHashRepository(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := infrastructure.NewHashRepository(db)
	ctx := context.Background()

	if _, ok, err := repo.Get(ctx, "p1", "project.toml"); ok || err != nil {
		t.Fatalf("Get empty = %v, %v", ok, err)
	}
	if err := repo.Put(ctx, "p1", "project.toml", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Put(ctx, "p1", "project.toml", "h2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Put(ctx, "p1", "tasks/a.toml", "ha"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Put(ctx, "p2", "project.toml", "other"); err != nil {
		t.Fatal(err)
	}
	if h, ok, err := repo.Get(ctx, "p1", "project.toml"); !ok || err != nil || h != "h2" {
		t.Errorf("Get = %q %v %v", h, ok, err)
	}
	all, err := repo.List(ctx, "p1")
	if err != nil || !reflect.DeepEqual(all, map[string]string{"project.toml": "h2", "tasks/a.toml": "ha"}) {
		t.Errorf("List = %v, %v", all, err)
	}
	if err := repo.Delete(ctx, "p1", "tasks/a.toml"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := repo.Get(ctx, "p1", "tasks/a.toml"); ok {
		t.Error("Delete did not remove the hash")
	}
}
