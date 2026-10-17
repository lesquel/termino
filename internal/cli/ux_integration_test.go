package cli_test

import (
	"strings"
	"testing"
)

func TestAddJoinsAllArgsAsTitle(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	out := mustRun(t, tmp, env, "add", "Fix", "login", "redirect")
	if !strings.Contains(out, "Created task WEB-1: Fix login redirect") {
		t.Errorf("add output = %q", out)
	}
	errOut := mustFail(t, tmp, env, "add")
	if !strings.Contains(errOut, "Usage: termino add <title> [flags]") {
		t.Errorf("missing title should show the usage line, got %q", errOut)
	}
}

func TestTaskListFilters(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	mustRun(t, tmp, env, "member", "add", "Ana")
	mustRun(t, tmp, env, "add", "Fix login", "--priority", "high", "--label", "bug")
	mustRun(t, tmp, env, "add", "Write docs", "--priority", "low")
	mustRun(t, tmp, env, "add", "Ship release")
	mustRun(t, tmp, env, "task", "assign", "WEB-1", "Ana")
	mustRun(t, tmp, env, "task", "move", "WEB-3", "In Progress")

	out := mustRun(t, tmp, env, "task", "list")
	for _, want := range []string{"WEB-1", "Fix login", "WEB-2", "WEB-3", "[In Progress]", "@Ana", "bug"} {
		if !strings.Contains(out, want) {
			t.Errorf("task list is missing %q:\n%s", want, out)
		}
	}
	if ls := mustRun(t, tmp, env, "ls"); ls != out {
		t.Errorf("ls differs from task list:\n%s\nvs\n%s", ls, out)
	}
	if ls := mustRun(t, tmp, env, "task", "ls"); ls != out {
		t.Errorf("task ls differs from task list:\n%s", ls)
	}

	cases := []struct {
		args []string
		want string
		not  []string
	}{
		{[]string{"--column", "in progress"}, "WEB-3", []string{"WEB-1", "WEB-2"}},
		{[]string{"--assignee", "ana"}, "WEB-1", []string{"WEB-2", "WEB-3"}},
		{[]string{"--label", "BUG"}, "WEB-1", []string{"WEB-2", "WEB-3"}},
		{[]string{"--priority", "low"}, "WEB-2", []string{"WEB-1", "WEB-3"}},
		{[]string{"--assignee", "none"}, "WEB-2", []string{"WEB-1"}},
	}
	for _, c := range cases {
		out := mustRun(t, tmp, env, append([]string{"task", "list"}, c.args...)...)
		if !strings.Contains(out, c.want) {
			t.Errorf("task list %v: missing %s:\n%s", c.args, c.want, out)
		}
		for _, n := range c.not {
			if strings.Contains(out, n) {
				t.Errorf("task list %v: should not list %s:\n%s", c.args, n, out)
			}
		}
	}
	if out := mustRun(t, tmp, env, "task", "list", "--label", "nope"); !strings.Contains(out, "No tasks match") {
		t.Errorf("empty filter result = %q", out)
	}
	mustFail(t, tmp, env, "task", "list", "--priority", "urgent")
}

func TestTaskListEmptyProjectHint(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	if out := mustRun(t, tmp, env, "task", "list"); !strings.Contains(out, "termino add") {
		t.Errorf("empty task list should hint at termino add, got %q", out)
	}
}

func TestUnknownSubcommandFailsWithSuggestion(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	errOut := mustFail(t, tmp, env, "task", "mvoe")
	if !strings.Contains(errOut, `unknown command "mvoe" for "termino task"`) || !strings.Contains(errOut, "move") {
		t.Errorf("stderr = %q, want unknown command with a suggestion", errOut)
	}
	for _, group := range []string{"project", "member", "sprint", "column", "widget"} {
		errOut := mustFail(t, tmp, env, group, "frobnicate")
		if !strings.Contains(errOut, "unknown command") {
			t.Errorf("%s frobnicate: stderr = %q", group, errOut)
		}
	}
	// A bare group still prints its help and succeeds.
	if out := mustRun(t, tmp, env, "task"); !strings.Contains(out, "Available Commands") {
		t.Errorf("bare task should print help, got %q", out)
	}
}

func TestArgErrorsShowUsageLine(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	errOut := mustFail(t, tmp, env, "task", "move", "WEB-1")
	if !strings.Contains(errOut, "Usage: termino task move <task> <column> [flags]") {
		t.Errorf("stderr = %q, want the usage line", errOut)
	}
	errOut = mustFail(t, tmp, env, "task", "list", "--bogus")
	if !strings.Contains(errOut, "Usage: termino task list") {
		t.Errorf("flag error stderr = %q, want the usage line", errOut)
	}
	// Runtime errors do not repeat the usage.
	mustRun(t, tmp, env, "add", "One")
	if errOut := mustFail(t, tmp, env, "task", "move", "WEB-1", "Nowhere"); strings.Contains(errOut, "Usage:") {
		t.Errorf("runtime error should not print usage: %q", errOut)
	}
}

func TestNotFoundErrorsListValidValues(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	mustRun(t, tmp, env, "add", "One")
	mustRun(t, tmp, env, "member", "add", "Ana")

	errOut := mustFail(t, tmp, env, "task", "move", "WEB-1", "Nowhere")
	if !strings.Contains(errOut, "Backlog, In Progress, Done") {
		t.Errorf("column not found should list columns: %q", errOut)
	}
	errOut = mustFail(t, tmp, env, "task", "assign", "WEB-1", "Zed")
	if !strings.Contains(errOut, "Ana") || strings.Contains(errOut, "System") {
		t.Errorf("member not found should list assignable members: %q", errOut)
	}
	errOut = mustFail(t, tmp, env, "task", "move", "ZZZ-1", "Done")
	if !strings.Contains(errOut, "termino task list") {
		t.Errorf("task not found should point to task list: %q", errOut)
	}
	errOut = mustFail(t, tmp, env, "project", "use", "NOPE")
	if !strings.Contains(errOut, "WEB") {
		t.Errorf("project not found should list keys: %q", errOut)
	}
	errOut = mustFail(t, tmp, env, "connect", "nope")
	if !strings.Contains(errOut, "kiro") {
		t.Errorf("unknown agent should list agents: %q", errOut)
	}
}

func TestProjectUseAndListMarksCurrent(t *testing.T) {
	tmp, env := isolatedEnv(t)
	mustRun(t, tmp, env, "project", "new", "--name", "Alpha", "--key", "ALP")
	mustRun(t, tmp, env, "project", "new", "--name", "Beta", "--key", "BET")

	out := mustRun(t, tmp, env, "project", "list")
	if l := lineWith(out, "BET"); !strings.HasPrefix(l, "* ") {
		t.Errorf("current project not marked: %q\n%s", l, out)
	}
	if l := lineWith(out, "ALP"); strings.HasPrefix(l, "* ") {
		t.Errorf("non-current project marked: %q", l)
	}

	if out := mustRun(t, tmp, env, "project", "use", "alp"); !strings.Contains(out, "ALP") {
		t.Errorf("project use output = %q", out)
	}
	if l := lineWith(mustRun(t, tmp, env, "project", "list"), "ALP"); !strings.HasPrefix(l, "* ") {
		t.Errorf("project use did not switch: %q", l)
	}
	if out := mustRun(t, tmp, env, "add", "Task in alpha"); !strings.Contains(out, "ALP-1") {
		t.Errorf("add after project use = %q", out)
	}
}

func TestNoProjectMessagesPointToSetup(t *testing.T) {
	tmp, env := isolatedEnv(t)
	if errOut := mustFail(t, tmp, env, "add", "x"); !strings.Contains(errOut, "termino setup") {
		t.Errorf("add without project: %q", errOut)
	}
	if out := mustRun(t, tmp, env, "project", "list"); !strings.Contains(out, "termino setup") {
		t.Errorf("empty project list: %q", out)
	}
}

func TestProjectNewPromptsOnSeparateLines(t *testing.T) {
	tmp, env := isolatedEnv(t)
	out, errOut, err := runWithStdin(t, tmp, env, "Web App\nweb\n", "project", "new")
	if err != nil {
		t.Fatalf("project new: %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(out, "Project name: \n") || !strings.Contains(out, "\nProject key") {
		t.Errorf("prompts should be on separate lines:\n%q", out)
	}
	if !strings.Contains(out, "Created project WEB (Web App)") {
		t.Errorf("multi-word name or key not read: %q", out)
	}
}
