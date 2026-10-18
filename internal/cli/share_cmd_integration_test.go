package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInit creates dir as a git work tree.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// copyTree copies the regular files under src into dst (a simulated clone).
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// sharedRepo sets up machine A: project WEB, user Ana, shared from a git
// repo holding one task.
func sharedRepo(t *testing.T) (repo string, env []string) {
	t.Helper()
	tmp, env := isolatedEnv(t)
	repo = filepath.Join(tmp, "repo")
	gitInit(t, repo)
	mustRun(t, repo, env, "setup", "--yes", "--name", "Ana", "--project", "Web", "--key", "WEB")
	out := mustRun(t, filepath.Join(repo), env, "share", "init")
	if !strings.Contains(out, repo) || !strings.Contains(out, "commit .termino/") {
		t.Errorf("share init output = %q", out)
	}
	mustRun(t, repo, env, "add", "Shared task")
	return repo, env
}

func TestShareInit_OutsideGitFails(t *testing.T) {
	tmp, env := newProjectEnv(t, "WEB", "kanban")
	errOut := mustFail(t, tmp, env, "share", "init")
	if !strings.Contains(errOut, "git repository") {
		t.Errorf("error = %q, want a clear git message", errOut)
	}
}

func TestShareInitStatusOff(t *testing.T) {
	repo, env := sharedRepo(t)
	if _, err := os.Stat(filepath.Join(repo, ".termino", "project.toml")); err != nil {
		t.Fatalf("project.toml not written: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".termino", "project.toml"))
	if !strings.Contains(string(data), `name = "Ana"`) {
		t.Errorf("user member not exported:\n%s", data)
	}

	broken := filepath.Join(repo, ".termino", "tasks", "bad.toml")
	if err := os.WriteFile(broken, []byte("<<<<<<< HEAD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustRun(t, filepath.Join(repo), env, "share", "status")
	for _, want := range []string{repo, "Files:", "3", "Last import:", "tasks/bad.toml", "conflict markers"} {
		if !strings.Contains(out, want) {
			t.Errorf("share status lacks %q:\n%s", want, out)
		}
	}

	out = mustRun(t, repo, env, "share", "off")
	if !strings.Contains(out, "Stopped sharing WEB") {
		t.Errorf("share off output = %q", out)
	}
	before, _ := filepath.Glob(filepath.Join(repo, ".termino", "tasks", "*.toml"))
	mustRun(t, repo, env, "add", "Local after off")
	after, _ := filepath.Glob(filepath.Join(repo, ".termino", "tasks", "*.toml"))
	if len(after) != len(before) {
		t.Errorf("share off still exports: %d files → %d", len(before), len(after))
	}
	if out := mustRun(t, repo, env, "share", "status"); !strings.Contains(out, "not shared") {
		t.Errorf("status after off = %q", out)
	}
}

func TestShareClone_NewMachineCreatesProject(t *testing.T) {
	repoA, _ := sharedRepo(t)
	tmpB, envB := isolatedEnv(t)
	repoB := filepath.Join(tmpB, "clone")
	gitInit(t, repoB)
	copyTree(t, repoA, repoB)

	out := mustRun(t, filepath.Join(repoB), envB, "ls")
	if !strings.Contains(out, "WEB-1") || !strings.Contains(out, "Shared task") {
		t.Fatalf("clone does not show the shared board:\n%s", out)
	}
	if out := mustRun(t, repoB, envB, "share", "status"); !strings.Contains(out, repoB) {
		t.Errorf("clone is not shared from its repo:\n%s", out)
	}
}

func TestShareClone_LinksExistingProjectAndAddsUser(t *testing.T) {
	repoA, _ := sharedRepo(t)
	tmpB, envB := isolatedEnv(t)
	repoB := filepath.Join(tmpB, "clone")
	gitInit(t, repoB)
	// B already has a WEB project (not shared) and the user Bob.
	mustRun(t, tmpB, envB, "setup", "--yes", "--name", "Bob", "--project", "Web", "--key", "WEB", "--no-associate")
	mustRun(t, tmpB, envB, "add", "Bob's local task")
	copyTree(t, repoA, repoB)

	out := mustRun(t, repoB, envB, "ls")
	if !strings.Contains(out, "Shared task") || !strings.Contains(out, "Bob's local task") {
		t.Fatalf("linked project lacks merged tasks:\n%s", out)
	}
	data, _ := os.ReadFile(filepath.Join(repoB, ".termino", "project.toml"))
	if !strings.Contains(string(data), `name = "Bob"`) {
		t.Errorf("config user not exported:\n%s", data)
	}
	files, _ := filepath.Glob(filepath.Join(repoB, ".termino", "tasks", "*.toml"))
	if len(files) != 2 {
		t.Errorf("task files = %d, want 2 (local task exported)", len(files))
	}
}

func TestShareClone_KeySharedFromOtherRootFails(t *testing.T) {
	repoA, env := sharedRepo(t)
	other := filepath.Join(filepath.Dir(repoA), "other")
	gitInit(t, other)
	copyTree(t, repoA, other)
	errOut := mustFail(t, other, env, "ls")
	if !strings.Contains(errOut, repoA) || !strings.Contains(errOut, other) {
		t.Errorf("error does not name both roots: %q", errOut)
	}
	if out := mustRun(t, repoA, env, "share", "status"); !strings.Contains(out, repoA) {
		t.Errorf("share root changed:\n%s", out)
	}
}

func TestSetupShareFlag(t *testing.T) {
	tmp, env := isolatedEnv(t)
	repo := filepath.Join(tmp, "repo")
	gitInit(t, repo)
	out := mustRun(t, repo, env, "setup", "--yes", "--name", "Ana", "--project", "Web", "--key", "WEB", "--share")
	if !strings.Contains(out, "Shared through "+filepath.Join(repo, ".termino")) {
		t.Errorf("setup output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".termino", "project.toml")); err != nil {
		t.Errorf("setup --share wrote no project.toml: %v", err)
	}

	plain := filepath.Join(tmp, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	out = mustRun(t, plain, env, "setup", "--yes", "--project", "Other", "--key", "OTH", "--share")
	if !strings.Contains(out, "Sharing skipped") {
		t.Errorf("setup outside git output = %q", out)
	}
}
