package application_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sharedomain "github.com/lesquel/termino/internal/domain"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	tasksourceapp "github.com/lesquel/termino/internal/features/tasksource/application"
	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
	"github.com/lesquel/termino/internal/features/tasksource/infrastructure/markdown"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

const (
	projectID = "p1"
	colTodo   = "c-todo"
	colDoing  = "c-doing"
	colDone   = "c-done"
)

var actor = sharedomain.Actor{ID: "cli", Name: "CLI", Kind: sharedomain.ActorKindHuman}

type env struct {
	svc    *tasksourceapp.SyncService
	taskUC *taskapp.TaskUseCases
	src    *markdown.Adapter
	dir    string
	logs   *syncBuffer
}

// syncBuffer is a goroutine-safe log sink so the watcher can log to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func newEnv(t *testing.T) env {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO projects (id, key, name, methodology, created_at) VALUES (?, 'P', 'P', 'kanban', ?)`, projectID, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	for i, id := range []string{colTodo, colDoing, colDone} {
		if _, err := db.Exec(`INSERT INTO columns (id, project_id, name, position, wip_limit) VALUES (?, ?, ?, ?, 0)`, id, projectID, id, i); err != nil {
			t.Fatalf("seed column: %v", err)
		}
	}
	taskRepo := taskinfra.NewTaskRepository(db)
	boardRepo := boardinfra.NewBoardRepository(db)
	taskUC := taskapp.NewTaskUseCases(taskRepo, taskinfra.NewCommentRepository(db), taskinfra.NewActivityRepository(db), boardRepo, events.NewBus())
	boardUC := boardapp.NewBoardUseCases(boardRepo, taskRepo)
	logs := &syncBuffer{}
	svc := tasksourceapp.NewSyncService(taskUC, boardUC).
		WithLogger(slog.New(slog.NewTextHandler(logs, nil)))
	return env{svc: svc, taskUC: taskUC, src: markdown.New(), dir: t.TempDir(), logs: logs}
}

func (e env) write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(e.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func (e env) importFile(t *testing.T, path string) (int, int) {
	t.Helper()
	sf := tasksourcedomain.SourceFile{Path: path, AdapterName: e.src.Name()}
	imported, updated, err := e.svc.ImportFromSource(context.Background(), actor, projectID, sf, e.src)
	if err != nil {
		t.Fatalf("ImportFromSource: %v", err)
	}
	return imported, updated
}

func (e env) tasks(t *testing.T) []taskdomain.Task {
	t.Helper()
	list, err := e.taskUC.ListTasks(context.Background(), taskdomain.TaskFilter{ProjectID: projectID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	return list
}

func TestImport_NewTasksLandInFirstColumnWithSanitizedTitles(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "- [ ] Fix \x1b[31mlogin\x1b]0;pwn\a\n- [ ] Write docs\n")

	imported, _ := e.importFile(t, path)
	if imported != 2 {
		t.Fatalf("imported = %d, want 2", imported)
	}
	for _, tk := range e.tasks(t) {
		if tk.ColumnID != colTodo {
			t.Errorf("task %q in column %q, want first column %q", tk.Title, tk.ColumnID, colTodo)
		}
		if strings.ContainsAny(tk.Title, "\x1b\a") || strings.Contains(tk.Title, "pwn") {
			t.Errorf("title not sanitized: %q", tk.Title)
		}
	}
}

func TestImport_CheckedBoxMovesCardToDone(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "- [ ] Ship it\n- [ ] Later\n")
	e.importFile(t, path)

	e.write(t, "tasks.md", "- [x] Ship it\n- [ ] Later\n")
	imported, updated := e.importFile(t, path)
	if imported != 0 || updated != 1 {
		t.Fatalf("re-import = (%d imported, %d updated), want (0, 1)", imported, updated)
	}
	for _, tk := range e.tasks(t) {
		want := colTodo
		if tk.Title == "Ship it" {
			want = colDone
		}
		if tk.ColumnID != want {
			t.Errorf("%q in %q, want %q", tk.Title, tk.ColumnID, want)
		}
	}

	// A third import is a no-op: no duplicates, no further moves.
	if imported, updated := e.importFile(t, path); imported != 0 || updated != 0 {
		t.Errorf("idempotent re-import = (%d, %d), want (0, 0)", imported, updated)
	}
	if n := len(e.tasks(t)); n != 2 {
		t.Errorf("task count = %d, want 2", n)
	}
}

func TestImport_CheckedAtFirstImportLandsInDone(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "- [x] Already done\n")
	e.importFile(t, path)
	if got := e.tasks(t); len(got) != 1 || got[0].ColumnID != colDone {
		t.Fatalf("tasks = %+v, want one task in Done", got)
	}
}

func TestImport_RelativeAndAbsolutePathsDoNotDuplicate(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "- [ ] One\n")
	e.importFile(t, path)

	t.Chdir(e.dir)
	if imported, _ := e.importFile(t, "tasks.md"); imported != 0 {
		t.Errorf("relative re-import created %d duplicate(s)", imported)
	}
}

func TestImport_DeletedTaskIsNotRecreated(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "- [ ] Drop me\n")
	e.importFile(t, path)
	tk := e.tasks(t)[0]
	if err := e.taskUC.DeleteTask(context.Background(), actor, tk.ID, true); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if imported, _ := e.importFile(t, path); imported != 0 {
		t.Errorf("deleted task was re-imported (%d)", imported)
	}
}

func TestExportTask_ChecksBoxWhenCardIsDone(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "# Plan\n- [ ] First\n- [ ] Second\n")
	e.importFile(t, path)
	var second taskdomain.Task
	for _, tk := range e.tasks(t) {
		if tk.Title == "Second" {
			second = tk
		}
	}
	ctx := context.Background()
	if err := e.taskUC.MoveTask(ctx, actor, second.ID, colDone, 1); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if err := e.svc.ExportTask(ctx, second.ID, []tasksourcedomain.TaskSource{e.src}); err != nil {
		t.Fatalf("ExportTask: %v", err)
	}
	data, _ := os.ReadFile(path)
	if got, want := string(data), "# Plan\n- [ ] First\n- [x] Second\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// waitFor polls cond until it holds or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func TestWatchImported_SyncsFileChangesAndStopsCleanly(t *testing.T) {
	e := newEnv(t)
	path := e.write(t, "tasks.md", "- [ ] Watch me\n")
	e.importFile(t, path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done, err := e.svc.WatchImported(ctx, actor, projectID, []tasksourcedomain.TaskSource{e.src})
	if err != nil {
		t.Fatalf("WatchImported: %v", err)
	}

	// Check the box and append a new item, as an agent editing the plan would.
	e.write(t, "tasks.md", "- [x] Watch me\n- [ ] Added later\n")
	synced := waitFor(t, 5*time.Second, func() bool {
		list := e.tasks(t)
		if len(list) != 2 {
			return false
		}
		for _, tk := range list {
			if tk.Title == "Watch me" && tk.ColumnID != colDone {
				return false
			}
			if tk.Title == "Added later" && tk.ColumnID != colTodo {
				return false
			}
		}
		return true
	})
	if !synced {
		t.Fatalf("watcher did not sync the file; tasks = %+v", e.tasks(t))
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop after context cancellation")
	}
}

func TestWatchImported_NoSourcesIsANoop(t *testing.T) {
	e := newEnv(t)
	done, err := e.svc.WatchImported(context.Background(), actor, projectID, []tasksourcedomain.TaskSource{e.src})
	if err != nil {
		t.Fatalf("WatchImported: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher with nothing to watch should finish immediately")
	}
}
