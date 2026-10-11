package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binaryPath holds the path to the compiled binary built once per test run.
var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary into a temp dir so tests can invoke it.
	dir, err := os.MkdirTemp("", "termino-cli-test-*")
	if err != nil {
		panic("build temp dir: " + err.Error())
	}
	defer os.RemoveAll(dir) //nolint:errcheck

	binaryPath = filepath.Join(dir, "termino")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/termino")
	cmd.Dir = projectRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build binary: " + err.Error())
	}

	os.Exit(m.Run())
}

// projectRoot resolves the repository root from within the internal/cli package.
func projectRoot() string {
	// Walk up from the test file's directory to find go.mod.
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// runTermino executes the binary with the given environment overrides and args.
// Returns stdout, stderr, and the exit error (nil on success).
func runTermino(t *testing.T, env []string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// TestHelp verifies that `termino --help` exits 0 and does NOT open any DB.
func TestHelp(t *testing.T) {
	// Use a fresh XDG dir so a pre-existing DB is not opened.
	tmp := t.TempDir()
	env := []string{
		"XDG_DATA_HOME=" + filepath.Join(tmp, "data"),
		"XDG_CONFIG_HOME=" + filepath.Join(tmp, "config"),
		"XDG_STATE_HOME=" + filepath.Join(tmp, "state"),
	}
	stdout, _, err := runTermino(t, env, "--help")
	if err != nil {
		t.Fatalf("--help exited non-zero: %v", err)
	}
	if !strings.Contains(stdout, "termino") {
		t.Errorf("--help output does not mention 'termino': %q", stdout)
	}
	// DB file must not have been created.
	dbPath := filepath.Join(tmp, "data", "termino", "termino.db")
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Error("--help opened the database, which it must not")
	}
}

// TestProjectNew verifies that `termino project new --name ... --key ... --methodology ...` creates a project.
func TestProjectNew(t *testing.T) {
	tmp := t.TempDir()
	env := []string{
		"XDG_DATA_HOME=" + filepath.Join(tmp, "data"),
		"XDG_CONFIG_HOME=" + filepath.Join(tmp, "config"),
		"XDG_STATE_HOME=" + filepath.Join(tmp, "state"),
	}
	stdout, stderr, err := runTermino(t, env, "project", "new",
		"--name", "Test Project",
		"--key", "TST",
		"--methodology", "kanban",
	)
	if err != nil {
		t.Fatalf("project new failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "TST") {
		t.Errorf("project new output does not mention key TST: %q", stdout)
	}
}

// TestAddTask verifies that `termino add "first task"` creates TST-1.
func TestAddTask(t *testing.T) {
	tmp := t.TempDir()
	env := []string{
		"XDG_DATA_HOME=" + filepath.Join(tmp, "data"),
		"XDG_CONFIG_HOME=" + filepath.Join(tmp, "config"),
		"XDG_STATE_HOME=" + filepath.Join(tmp, "state"),
	}
	// Create a project first.
	if _, _, err := runTermino(t, env, "project", "new",
		"--name", "Task Test",
		"--key", "TTT",
		"--methodology", "kanban",
	); err != nil {
		t.Fatalf("project new: %v", err)
	}

	stdout, stderr, err := runTermino(t, env, "add", "first task")
	if err != nil {
		t.Fatalf("add task failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "TTT-1") {
		t.Errorf("add task output does not mention TTT-1: %q", stdout)
	}
}

// TestTheme verifies that `termino theme spooky` updates the config.
func TestTheme(t *testing.T) {
	tmp := t.TempDir()
	env := []string{
		"XDG_DATA_HOME=" + filepath.Join(tmp, "data"),
		"XDG_CONFIG_HOME=" + filepath.Join(tmp, "config"),
		"XDG_STATE_HOME=" + filepath.Join(tmp, "state"),
	}
	stdout, stderr, err := runTermino(t, env, "theme", "spooky")
	if err != nil {
		t.Fatalf("theme failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "spooky") {
		t.Errorf("theme output does not mention 'spooky': %q", stdout)
	}
}
