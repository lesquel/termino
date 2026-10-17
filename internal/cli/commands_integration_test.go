package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustRun runs the binary in dir and fails the test on a non-zero exit.
func mustRun(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	out, errOut, err := runIn(t, dir, env, args...)
	if err != nil {
		t.Fatalf("termino %v: %v\nstdout: %s\nstderr: %s", args, err, out, errOut)
	}
	return out
}

// mustFail runs the binary and fails the test when it succeeds.
func mustFail(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	out, errOut, err := runIn(t, dir, env, args...)
	if err == nil {
		t.Fatalf("termino %v succeeded, want an error\nstdout: %s", args, out)
	}
	return errOut
}

// lineWith returns the first output line containing needle, or "".
func lineWith(out, needle string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

func newProjectEnv(t *testing.T, key, methodology string) (string, []string) {
	t.Helper()
	tmp, env := isolatedEnv(t)
	mustRun(t, tmp, env, "project", "new", "--name", "P "+key, "--key", key, "--methodology", methodology)
	return tmp, env
}

func TestMemberCommands(t *testing.T) {
	tmp, env := isolatedEnv(t)

	if out := mustRun(t, tmp, env, "member", "add", "Ana"); !strings.Contains(out, "Ana") {
		t.Errorf("member add output = %q", out)
	}
	mustRun(t, tmp, env, "member", "add", "kiro-bot", "--kind", "agent")
	mustRun(t, tmp, env, "member", "add", "\x1b]0;pwn\x07Bob")
	mustFail(t, tmp, env, "member", "add", "X", "--kind", "robot")
	mustFail(t, tmp, env, "member", "add", "\x1b[2J")

	out := mustRun(t, tmp, env, "member", "list")
	if l := lineWith(out, "Ana"); !strings.Contains(l, "human") {
		t.Errorf("Ana line = %q, want kind human\n%s", l, out)
	}
	if l := lineWith(out, "kiro-bot"); !strings.Contains(l, "agent") {
		t.Errorf("kiro-bot line = %q, want kind agent\n%s", l, out)
	}
	if lineWith(out, "Bob") == "" || strings.Contains(out, "\x1b") {
		t.Errorf("sanitized member missing or escape leaked: %q", out)
	}

	mustRun(t, tmp, env, "member", "remove", "Ana")
	if out := mustRun(t, tmp, env, "member", "list"); lineWith(out, "Ana") != "" {
		t.Errorf("Ana still listed after remove:\n%s", out)
	}
	mustFail(t, tmp, env, "member", "remove", "nobody-here")
}

func TestColumnCommands(t *testing.T) {
	tmp, env := newProjectEnv(t, "COL", "kanban")

	mustRun(t, tmp, env, "column", "add", "Review")
	mustRun(t, tmp, env, "column", "rename", "Review", "QA")
	mustRun(t, tmp, env, "column", "wip", "In Progress", "2")
	mustFail(t, tmp, env, "column", "wip", "In Progress", "-1")
	mustRun(t, tmp, env, "column", "reorder", "Backlog", "QA", "In Progress", "Done")
	mustFail(t, tmp, env, "column", "reorder", "Backlog", "Done")

	out := mustRun(t, tmp, env, "column", "list")
	order := []string{"Backlog", "QA", "In Progress", "Done"}
	last := -1
	for _, name := range order {
		i := strings.Index(out, name)
		if i < 0 || i < last {
			t.Fatalf("column order wrong, want %v:\n%s", order, out)
		}
		last = i
	}
	if l := lineWith(out, "In Progress"); !strings.Contains(l, "WIP 2") {
		t.Errorf("In Progress line = %q, want WIP 2", l)
	}

	mustRun(t, tmp, env, "add", "keep me", "--column", "")
	mustFail(t, tmp, env, "column", "delete", "Backlog") // still has a task
	mustRun(t, tmp, env, "column", "delete", "QA")
	if out := mustRun(t, tmp, env, "column", "list"); lineWith(out, "QA") != "" {
		t.Errorf("QA still listed after delete:\n%s", out)
	}
}

func TestSprintCommands(t *testing.T) {
	tmp, env := newProjectEnv(t, "SPR", "scrum")

	mustRun(t, tmp, env, "sprint", "new", "Sprint 1", "--start", "2026-10-01", "--end", "2026-10-14")
	mustRun(t, tmp, env, "sprint", "new", "Sprint 2")
	mustFail(t, tmp, env, "sprint", "new", "Bad", "--start", "2026-10-14", "--end", "2026-10-01")

	out := mustRun(t, tmp, env, "sprint", "list")
	if l := lineWith(out, "Sprint 1"); !strings.Contains(l, "draft") || !strings.Contains(l, "2026-10-01") {
		t.Errorf("Sprint 1 line = %q", l)
	}

	mustRun(t, tmp, env, "sprint", "start", "Sprint 1")
	mustFail(t, tmp, env, "sprint", "start", "Sprint 2") // one active sprint per project
	if l := lineWith(mustRun(t, tmp, env, "sprint", "list"), "Sprint 1"); !strings.Contains(l, "active") {
		t.Errorf("Sprint 1 after start = %q", l)
	}
	mustRun(t, tmp, env, "sprint", "close", "Sprint 1")
	if l := lineWith(mustRun(t, tmp, env, "sprint", "list"), "Sprint 1"); !strings.Contains(l, "closed") {
		t.Errorf("Sprint 1 after close = %q", l)
	}
	mustRun(t, tmp, env, "sprint", "start", "Sprint 2")
}

func TestTaskMoveAssignDoneAndMyTasks(t *testing.T) {
	tmp, env := newProjectEnv(t, "TSK", "kanban")
	mustRun(t, tmp, env, "member", "add", "Ana")
	mustRun(t, tmp, env, "add", "first task")
	if err := os.WriteFile(filepath.Join(tmp, "tasks.md"), []byte("- [ ] from plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, tmp, env, "import", "tasks.md")

	mustRun(t, tmp, env, "task", "move", "TSK-1", "In Progress")
	mustFail(t, tmp, env, "task", "move", "TSK-1", "Nowhere")
	mustFail(t, tmp, env, "task", "move", "TSK-99", "Done")
	mustRun(t, tmp, env, "task", "assign", "1", "Ana")
	mustRun(t, tmp, env, "task", "assign", "TSK-2", "cli")

	out := mustRun(t, tmp, env, "my-tasks", "--member", "Ana")
	if l := lineWith(out, "TSK-1"); !strings.Contains(l, "first task") || !strings.Contains(l, "In Progress") {
		t.Errorf("my-tasks --member Ana = %q", out)
	}
	if lineWith(out, "TSK-2") != "" {
		t.Errorf("Ana's list shows another member's task:\n%s", out)
	}
	if out := mustRun(t, tmp, env, "my-tasks"); lineWith(out, "TSK-2") == "" || lineWith(out, "TSK-1") != "" {
		t.Errorf("default my-tasks (current member) = %q", out)
	}

	mustRun(t, tmp, env, "task", "done", "TSK-2")
	data, _ := os.ReadFile(filepath.Join(tmp, "tasks.md"))
	if !strings.Contains(string(data), "- [x] from plan") {
		t.Errorf("task done did not check the source box:\n%s", data)
	}
	if l := lineWith(mustRun(t, tmp, env, "my-tasks"), "TSK-2"); !strings.Contains(l, "Done") {
		t.Errorf("TSK-2 after done = %q", l)
	}
	mustRun(t, tmp, env, "task", "assign", "TSK-1", "none")
	if out := mustRun(t, tmp, env, "my-tasks", "--member", "Ana"); lineWith(out, "TSK-1") != "" {
		t.Errorf("unassigned task still listed:\n%s", out)
	}
}
