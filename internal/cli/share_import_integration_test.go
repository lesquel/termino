package cli_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	shareapp "github.com/lesquel/termino/internal/features/share/application"
	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
	sprintinfra "github.com/lesquel/termino/internal/features/sprint/infrastructure"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

const foreignTaskID = "0f0f0f0f-1111-2222-3333-444444444444"

// sharedProjectEnv creates project WEB shared from <tmp>/repo and exports it.
func sharedProjectEnv(t *testing.T) (tmp string, env []string, root string) {
	t.Helper()
	tmp, env = newProjectEnv(t, "WEB", "kanban")
	root = filepath.Join(tmp, "repo")
	db, err := sqlite.Open(filepath.Join(tmp, "data", "termino", "termino.db"))
	if err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := db.QueryRow(`SELECT id FROM projects WHERE key = 'WEB'`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := testShareService(db).Init(context.Background(), projectID, root); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	mustRun(t, tmp, env, "add", "Mine")
	return tmp, env, root
}

// testShareService wires a share service on db like the container does.
func testShareService(db *sql.DB) *shareapp.ShareService {
	return shareapp.NewShareService(shareapp.Deps{
		Projects: projectinfra.NewProjectRepository(db),
		Board:    boardinfra.NewBoardRepository(db),
		Tasks:    taskinfra.NewTaskRepository(db),
		Members:  memberinfra.NewMemberRepository(db),
		Labels:   taskinfra.NewLabelRepository(db),
		Sprints:  sprintinfra.NewSprintRepository(db),
		Files:    shareinfra.FileStore{},
		Hashes:   shareinfra.NewHashRepository(db),
		Codec:    shareinfra.TOMLCodec{},
		State:    shareinfra.NewImportStateRepository(db),
	})
}

// writeForeignTask drops a task file as a teammate's git pull would.
func writeForeignTask(t *testing.T, root, title string) {
	t.Helper()
	dir := filepath.Join(root, ".termino", "tasks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `version = 1
id = "` + foreignTaskID + `"
number = 9
title = "` + title + `"
column = "In Progress"
position = 1
priority = "high"
created_at = 2026-10-17T18:00:00Z
updated_at = 2026-10-17T18:10:00Z
updated_by = "Zoe"
`
	if err := os.WriteFile(filepath.Join(dir, foreignTaskID+".toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestShareImport_EveryCommandImportsFirst(t *testing.T) {
	tmp, env, root := sharedProjectEnv(t)
	writeForeignTask(t, root, "From Zoe")
	out := mustRun(t, tmp, env, "ls")
	if !strings.Contains(out, "From Zoe") || !strings.Contains(out, "WEB-9") {
		t.Errorf("ls did not import the shared file:\n%s", out)
	}
	writeForeignTask(t, root, "From Zoe, edited")
	if out := mustRun(t, tmp, env, "task", "list"); !strings.Contains(out, "From Zoe, edited") {
		t.Errorf("task list did not import the edit:\n%s", out)
	}
	if err := os.Remove(filepath.Join(root, ".termino", "tasks", foreignTaskID+".toml")); err != nil {
		t.Fatal(err)
	}
	if out := mustRun(t, tmp, env, "ls"); strings.Contains(out, "From Zoe") {
		t.Errorf("deleted shared file still listed:\n%s", out)
	}
}

func TestShareImport_MCPStartImportsWithoutTouchingStdout(t *testing.T) {
	tmp, env, root := sharedProjectEnv(t)
	writeForeignTask(t, root, "Via MCP")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "mcp", "--read-only")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}` + "\n")
	var stdout strings.Builder
	cmd.Stdout = &stdout
	_ = cmd.Run()
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line != "" && !strings.HasPrefix(line, "{") {
			t.Errorf("non JSON-RPC stdout line: %q", line)
		}
	}
	db, err := sqlite.Open(filepath.Join(tmp, "data", "termino", "termino.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title string
	if err := db.QueryRow(`SELECT title FROM tasks WHERE id = ?`, foreignTaskID).Scan(&title); err != nil || title != "Via MCP" {
		t.Errorf("MCP start did not import: %q %v", title, err)
	}
}
