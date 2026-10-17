package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestColumnNamesAreUniqueIgnoringCase(t *testing.T) {
	tmp, env := newProjectEnv(t, "COL", "kanban")
	mustRun(t, tmp, env, "column", "add", "Review")
	errOut := mustFail(t, tmp, env, "column", "add", "review")
	if !strings.Contains(errOut, "already exists") {
		t.Errorf("duplicate column error = %q", errOut)
	}
	mustFail(t, tmp, env, "column", "rename", "Review", "DONE")
}

func TestDuplicateMemberIsAFriendlyError(t *testing.T) {
	tmp, env := isolatedEnv(t)
	mustRun(t, tmp, env, "member", "add", "Ana")
	errOut := mustFail(t, tmp, env, "member", "add", "Ana")
	if strings.Contains(errOut, "UNIQUE") || !strings.Contains(errOut, "already exists") {
		t.Errorf("duplicate member error = %q", errOut)
	}
}

func TestCommandsUseProjectAssociatedWithCwd(t *testing.T) {
	tmp, env := isolatedEnv(t)
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustRun(t, tmp, env, "project", "new", "--name", "Repo", "--key", "REP", "--path", repo)
	// The current project is now OTH; inside repo/ the association wins.
	mustRun(t, tmp, env, "project", "new", "--name", "Other", "--key", "OTH")

	out := mustRun(t, filepath.Join(repo, "sub"), env, "add", "From repo")
	if !strings.Contains(out, "REP-1") {
		t.Errorf("add inside the associated dir should use REP, got %q", out)
	}
	out = mustRun(t, tmp, env, "add", "From elsewhere")
	if !strings.Contains(out, "OTH-1") {
		t.Errorf("add outside should use the current project OTH, got %q", out)
	}
	out = mustRun(t, repo, env, "column", "list")
	if !strings.Contains(out, "1 task(s)") {
		t.Errorf("column list inside repo should show REP's task:\n%s", out)
	}
}

func TestThemeWithoutArgsListsThemes(t *testing.T) {
	tmp, env := isolatedEnv(t)
	out := mustRun(t, tmp, env, "theme")
	if !strings.Contains(out, "default") || !strings.Contains(out, "spooky") {
		t.Errorf("theme list = %q", out)
	}
	if l := lineWith(out, "default"); !strings.Contains(l, "*") {
		t.Errorf("current theme not marked: %q", out)
	}
	mustRun(t, tmp, env, "theme", "spooky")
	if l := lineWith(mustRun(t, tmp, env, "theme"), "spooky"); !strings.Contains(l, "*") {
		t.Errorf("spooky not marked current after switching")
	}
	// Themes that do not exist are rejected.
	mustFail(t, tmp, env, "theme", "ocean")
}

func TestColumnListFlagsWIPExceeded(t *testing.T) {
	tmp, env := newProjectEnv(t, "WIP", "kanban")
	mustRun(t, tmp, env, "add", "One")
	mustRun(t, tmp, env, "add", "Two")
	first := strings.Fields(strings.TrimSpace(mustRun(t, tmp, env, "column", "list")))[0]
	mustRun(t, tmp, env, "column", "wip", first, "1")
	out := mustRun(t, tmp, env, "column", "list")
	if l := lineWith(out, first); !strings.Contains(l, "exceeded") {
		t.Errorf("column over its WIP limit not flagged:\n%s", out)
	}
}

func TestAddWithLabels(t *testing.T) {
	tmp, env := newProjectEnv(t, "LAB", "kanban")
	out := mustRun(t, tmp, env, "add", "Fix login", "--label", "bug", "--label", "UI,bug")
	if !strings.Contains(out, "[UI, bug]") {
		t.Errorf("add should report the attached labels once each:\n%s", out)
	}
	out = mustRun(t, tmp, env, "add", "Fix logout", "--label", "Bug")
	if !strings.Contains(out, "[bug]") {
		t.Errorf("an existing label should be reused (case-insensitive):\n%s", out)
	}
}

func TestWidgetInstallShellFlag(t *testing.T) {
	tmp, env := isolatedEnv(t)
	home := filepath.Join(tmp, "home")
	mustRun(t, tmp, env, "widget", "install", "--shell", "zsh")
	rc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil || !strings.Contains(string(rc), "termino-widget-start") {
		t.Errorf("--shell zsh did not write .zshrc: %v %s", err, rc)
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Errorf("--shell zsh touched .bashrc")
	}
	if errOut := mustFail(t, tmp, env, "widget", "install", "--shell", "fish"); !strings.Contains(errOut, "fish") {
		t.Errorf("fish error should name the shell: %q", errOut)
	}
	mustFail(t, tmp, env, "widget", "install", "--shell", "tcsh")
}

func TestTaskSprintPutsTaskInSprint(t *testing.T) {
	tmp, env := newProjectEnv(t, "SPR", "scrum")
	mustRun(t, tmp, env, "add", "Story")
	mustRun(t, tmp, env, "add", "Story two")
	mustRun(t, tmp, env, "sprint", "new", "S1")
	if out := mustRun(t, tmp, env, "task", "sprint", "SPR-1", "S1"); !strings.Contains(out, "SPR-1") {
		t.Errorf("task sprint output = %q", out)
	}
	mustRun(t, tmp, env, "sprint", "add", "S1", "SPR-2")
	out := mustRun(t, tmp, env, "sprint", "list")
	if l := lineWith(out, "S1"); !strings.Contains(l, "2 task(s)") {
		t.Errorf("sprint S1 should hold 2 tasks:\n%s", out)
	}
	mustFail(t, tmp, env, "task", "sprint", "SPR-1", "nope")
}

func TestSetupRegistersUserAsMemberForMyTasks(t *testing.T) {
	tmp, env := isolatedEnv(t)
	// name, project name, key, associate cwd, (no agents found), widget, theme
	answers := "Leo\nDemo\nDEM\nn\nn\nspooky\n"
	if out, errOut, err := runWithStdin(t, tmp, env, answers, "setup"); err != nil {
		t.Fatalf("setup: %v\n%s\n%s", err, out, errOut)
	}
	if l := lineWith(mustRun(t, tmp, env, "member", "list"), "Leo"); !strings.Contains(l, "human") {
		t.Fatalf("setup user not registered as a human member")
	}
	mustRun(t, tmp, env, "add", "Mine")
	mustRun(t, tmp, env, "task", "assign", "DEM-1", "Leo")
	if out := mustRun(t, tmp, env, "my-tasks"); !strings.Contains(out, "DEM-1") {
		t.Errorf("my-tasks should default to the setup user:\n%s", out)
	}
}
