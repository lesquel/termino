package infrastructure_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
)

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWatch_DebouncesBurstsAndSeesNewTasksDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".termino")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done, err := shareinfra.Watch(ctx, dir, 100*time.Millisecond, func() { calls.Add(1) }, nil)
	if err != nil {
		t.Fatal(err)
	}

	store := shareinfra.FileStore{}
	for i := 0; i < 5; i++ {
		if err := store.Write(filepath.Join(dir, "project.toml"), []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "first change", func() bool { return calls.Load() >= 1 })
	time.Sleep(300 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("a burst of writes ran onChange %d times, want 1", n)
	}

	// The tasks directory is created after the watcher started.
	if err := store.Write(filepath.Join(dir, "tasks", "a.toml"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "tasks dir change", func() bool { return calls.Load() >= 2 })
	before := calls.Load()
	if err := store.Write(filepath.Join(dir, "tasks", "b.toml"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "task file change", func() bool { return calls.Load() > before })

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not stop after cancel")
	}
}
