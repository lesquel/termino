package cli_test

import (
	"strings"
	"testing"
)

// The reported case: the only task sits in Backlog. The widget must list it
// with the column counts instead of claiming there is nothing to do.
func TestWidgetShowsBacklogTasks(t *testing.T) {
	tmp, env := newProjectEnv(t, "PROJ", "kanban")
	out := mustRun(t, tmp, env, "widget")
	if !strings.Contains(out, `No tasks yet — termino add "..."`) {
		t.Fatalf("empty project hint missing:\n%s", out)
	}

	mustRun(t, tmp, env, "add", "Design", "the", "landing", "page")
	out = mustRun(t, tmp, env, "widget")
	for _, want := range []string{"P PROJ (PROJ)", "Backlog 1 · In Progress 0 · Done 0", "PROJ-1", "Design the landing page", "termino board"} {
		if !strings.Contains(out, want) {
			t.Errorf("widget output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no active tasks") || strings.Contains(out, "\x1b[") {
		t.Errorf("unexpected text or colour codes on a pipe:\n%q", out)
	}
}
