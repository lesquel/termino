package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sharedomain "github.com/lesquel/termino/internal/domain"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasksourceapp "github.com/lesquel/termino/internal/features/tasksource/application"
	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
	"github.com/lesquel/termino/internal/features/tasksource/infrastructure/markdown"
)

// importOne imports a one-item markdown checklist into the env's project and
// wires OnTaskDone to the sync service export, as `termino mcp` does.
func importOne(t *testing.T, env *sqliteEnv) (taskID, path string) {
	t.Helper()
	ctx := context.Background()
	path = filepath.Join(t.TempDir(), "tasks.md")
	if err := os.WriteFile(path, []byte("# Plan\n\n- [ ] Fix login\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sources := []tasksourcedomain.TaskSource{markdown.New()}
	svc := tasksourceapp.NewSyncService(env.deps.TaskUC, env.deps.BoardUC)
	human := sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
	sf := tasksourcedomain.SourceFile{Path: path, AdapterName: markdown.New().Name()}
	if _, _, err := svc.ImportFromSource(ctx, human, env.project.ID, sf, sources[0]); err != nil {
		t.Fatalf("import: %v", err)
	}
	env.deps.OnTaskDone = func(ctx context.Context, id string) error {
		return svc.ExportTask(ctx, id, sources)
	}
	all, err := env.deps.TaskUC.ListTasks(ctx, taskdomain.TaskFilter{ProjectID: env.project.ID})
	if err != nil || len(all) != 1 {
		t.Fatalf("imported tasks = %d (err %v), want 1", len(all), err)
	}
	return all[0].ID, path
}

func assertChecked(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "- [x] Fix login") {
		t.Fatalf("source checkbox not checked:\n%s", data)
	}
}

func TestCompleteTask_SQLite_ChecksSourceBox(t *testing.T) {
	env := newSQLiteEnv(t, "")
	taskID, path := importOne(t, &env)
	c := connect(t, env.deps, "kiro")

	callTool(t, c, "complete_task", map[string]any{"task_id": taskID, "project_id": env.project.ID})
	assertChecked(t, path)
}

func TestMoveTaskToDone_SQLite_ChecksSourceBox(t *testing.T) {
	env := newSQLiteEnv(t, "")
	taskID, path := importOne(t, &env)
	c := connect(t, env.deps, "kiro")

	board, err := env.deps.BoardUC.GetBoard(context.Background(), env.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := boarddomain.DoneColumn(board.Columns)
	callTool(t, c, "move_task", map[string]any{"task_id": taskID, "target_column_id": done.ID})
	assertChecked(t, path)
}
