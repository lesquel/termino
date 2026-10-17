package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectLines returns the data lines of `project list` (one per project).
func projectLines(t *testing.T, dir string, env []string) []string {
	t.Helper()
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(mustRun(t, dir, env, "project", "list")), "\n") {
		if strings.Contains(l, "kanban") || strings.Contains(l, "scrum") {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestSetupYesUsesDefaults(t *testing.T) {
	tmp, env := isolatedEnv(t)
	env = append(env, "USER=tester")
	dir := filepath.Join(tmp, "web-app")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	out := mustRun(t, dir, env, "setup", "--yes")
	if !strings.Contains(out, "Project created: web-app (WA)") || !strings.Contains(out, "termino board") {
		t.Fatalf("setup --yes output:\n%s", out)
	}
	if l := lineWith(mustRun(t, dir, env, "member", "list"), "tester"); !strings.Contains(l, "human") {
		t.Fatalf("$USER not registered as a human member")
	}
	if strings.Contains(mustRun(t, dir, env, "member", "list"), " human  human") {
		t.Fatal("placeholder member named human was created")
	}
	// The cwd was associated, so commands inside it resolve the project.
	if out := mustRun(t, dir, env, "add", "First"); !strings.Contains(out, "WA-1") {
		t.Fatalf("add in associated dir: %q", out)
	}
}

func TestSetupYesRerunEditsWithoutDuplicating(t *testing.T) {
	tmp, env := isolatedEnv(t)
	mustRun(t, tmp, env, "setup", "--yes", "--name", "Ana", "--project", "Web", "--key", "WEB", "--no-associate")
	out := mustRun(t, tmp, env, "setup", "--yes", "--project", "Web App", "--methodology", "scrum", "--theme", "spooky")
	if !strings.Contains(out, "Project updated: Web App (WEB)") {
		t.Fatalf("re-run output:\n%s", out)
	}
	lines := projectLines(t, tmp, env)
	if len(lines) != 1 || !strings.Contains(lines[0], "Web App") || !strings.Contains(lines[0], "scrum") {
		t.Fatalf("project list after edit:\n%v", lines)
	}
	if !strings.Contains(mustRun(t, tmp, env, "theme"), "spooky") {
		t.Fatal("theme not saved")
	}
	// A new key adds a project; naming an existing key edits that one.
	mustRun(t, tmp, env, "setup", "--yes", "--project", "Api", "--key", "API")
	mustRun(t, tmp, env, "setup", "--yes", "--project", "Website", "--key", "WEB")
	lines = projectLines(t, tmp, env)
	if len(lines) != 2 || lineWith(strings.Join(lines, "\n"), "WEB") == "" ||
		!strings.Contains(lineWith(strings.Join(lines, "\n"), "WEB"), "Website") {
		t.Fatalf("projects after add + edit:\n%v", lines)
	}
}

func TestSetupYesValidatesBeforeWriting(t *testing.T) {
	tmp, env := isolatedEnv(t)
	errOut := mustFail(t, tmp, env, "setup", "--yes", "--name", "Ana", "--project", "Web", "--key", "w1", "--theme", "nope")
	if !strings.Contains(errOut, "2-5 uppercase letters") || !strings.Contains(errOut, `unknown theme "nope"`) || !strings.Contains(errOut, "nothing was changed") {
		t.Fatalf("validation errors:\n%s", errOut)
	}
	if _, err := os.Stat(filepath.Join(tmp, "config", "termino", "config.toml")); err == nil {
		cfg, _ := os.ReadFile(filepath.Join(tmp, "config", "termino", "config.toml"))
		if strings.Contains(string(cfg), "Ana") {
			t.Fatalf("config written despite validation error:\n%s", cfg)
		}
	}
	if out := mustRun(t, tmp, env, "project", "list"); !strings.Contains(out, "termino setup") {
		t.Fatalf("a project was created: %s", out)
	}
}

func TestSetupLinePromptsRerunEdit(t *testing.T) {
	tmp, env := isolatedEnv(t)
	if out, errOut, err := runWithStdin(t, tmp, env, "Leo\nDemo\nDEM\n\nn\n\nn\n", "setup"); err != nil {
		t.Fatalf("setup: %v\n%s\n%s", err, out, errOut)
	}
	out, errOut, err := runWithStdin(t, tmp, env, "e\n\nDemo Two\n\n\n\nn\n", "setup")
	if err != nil {
		t.Fatalf("re-run: %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(out, "Your name [Leo]") || !strings.Contains(out, "Project updated: Demo Two (DEM)") {
		t.Fatalf("re-run output:\n%s", out)
	}
	if lines := projectLines(t, tmp, env); len(lines) != 1 {
		t.Fatalf("re-run duplicated the project:\n%v", lines)
	}
}
