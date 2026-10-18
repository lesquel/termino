package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesquel/termino/internal/platform/sqlite"
)

// TestShareExport_CLIMutationsWriteTaskFiles checks the container wiring: a
// project with a share root gets its task files written by plain CLI
// commands, and one without a share root gets nothing.
func TestShareExport_CLIMutationsWriteTaskFiles(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	root := filepath.Join(tmp, "repo")
	db, err := sqlite.Open(filepath.Join(tmp, "data", "termino", "termino.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE projects SET share_root = ? WHERE key = 'WEB'`, root); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	mustRun(t, tmp, env, "add", "Shared", "task")
	mustRun(t, tmp, env, "task", "move", "WEB-1", "Done")

	files, err := filepath.Glob(filepath.Join(root, ".termino", "tasks", "*.toml"))
	if err != nil || len(files) != 1 {
		t.Fatalf("task files = %v, %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`title = "Shared task"`, `column = "Done"`, "number = 1"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("task file misses %q:\n%s", want, data)
		}
	}
}

func TestShareExport_UnsharedProjectWritesNothing(t *testing.T) {
	tmp, env := newProjectEnv(t, "LOC", "kanban")
	mustRun(t, tmp, env, "add", "Local")
	if _, err := os.Stat(filepath.Join(tmp, ".termino")); !os.IsNotExist(err) {
		t.Errorf(".termino created for an unshared project: %v", err)
	}
}
