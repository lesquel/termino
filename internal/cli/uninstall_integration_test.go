package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runWithStdin executes the binary in dir feeding stdin.
func runWithStdin(t *testing.T, dir string, env []string, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// kiroEnv prepares an isolated HOME with a ~/.kiro dir and a project dir.
func kiroEnv(t *testing.T) (tmp, home, project string, env []string) {
	t.Helper()
	tmp, env = isolatedEnv(t)
	home = filepath.Join(tmp, "home")
	if err := os.MkdirAll(filepath.Join(home, ".kiro", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	project = filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	return tmp, home, project, env
}

func readRegistry(t *testing.T, tmp string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tmp, "state", "termino", "installed.json"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func TestUninstallKeepsSharedKiroConfig(t *testing.T) {
	tmp, home, project, env := kiroEnv(t)
	mcpPath := filepath.Join(home, ".kiro", "settings", "mcp.json")
	shared := `{"mcpServers":{"other":{"command":"other-server","args":[]}},"theme":"dark"}`
	if err := os.WriteFile(mcpPath, []byte(shared), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, project, env, "connect", "kiro")

	if out, errOut, err := runWithStdin(t, project, env, strings.Repeat("y\n", 10), "uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s\n%s", err, out, errOut)
	}
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("uninstall deleted the shared Kiro config: %v", err)
	}
	cfg := string(data)
	if !strings.Contains(cfg, "other-server") || !strings.Contains(cfg, `"theme"`) {
		t.Errorf("other Kiro settings lost: %s", cfg)
	}
	if strings.Contains(cfg, `"termino"`) {
		t.Errorf("termino entry still in Kiro config: %s", cfg)
	}
	if reg := readRegistry(t, tmp); strings.Contains(reg, "termino-steering") {
		t.Errorf("removed items still recorded: %s", reg)
	}
}

func TestUninstallKeepsDeclinedItems(t *testing.T) {
	tmp, home, project, env := kiroEnv(t)
	mustRun(t, project, env, "widget", "install")
	mustRun(t, project, env, "connect", "kiro")

	if out, errOut, err := runWithStdin(t, project, env, strings.Repeat("n\n", 10), "uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s\n%s", err, out, errOut)
	}
	reg := readRegistry(t, tmp)
	for _, marker := range []string{"termino-widget-start", "termino-steering", `"Marker": "termino"`} {
		if !strings.Contains(reg, marker) {
			t.Errorf("declined item %s dropped from registry: %s", marker, reg)
		}
	}
	rc, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if !strings.Contains(string(rc), "termino-widget-start") {
		t.Errorf("declined widget block was removed: %s", rc)
	}
}

func TestReinstallReRegistersExistingArtifacts(t *testing.T) {
	tmp, _, project, env := kiroEnv(t)
	mustRun(t, project, env, "widget", "install")
	mustRun(t, project, env, "connect", "kiro")
	if err := os.Remove(filepath.Join(tmp, "state", "termino", "installed.json")); err != nil {
		t.Fatal(err)
	}

	mustRun(t, project, env, "widget", "install")
	mustRun(t, project, env, "connect", "kiro")
	reg := readRegistry(t, tmp)
	for _, marker := range []string{"termino-widget-start", "termino-steering", `"Marker": "termino"`} {
		if !strings.Contains(reg, marker) {
			t.Errorf("existing artifact %s not re-registered: %s", marker, reg)
		}
	}

	// Re-registering must not duplicate entries.
	mustRun(t, project, env, "connect", "kiro")
	if n := strings.Count(readRegistry(t, tmp), "termino-steering"); n != 1 {
		t.Errorf("steering recorded %d times, want 1", n)
	}
}

func TestUninstallPrintsBinaryPathAndPurgesSQLiteSidecars(t *testing.T) {
	tmp, _, project, env := kiroEnv(t)
	dataDir := filepath.Join(tmp, "data", "termino")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"termino.db", "termino.db-shm", "termino.db-wal"} {
		if err := os.WriteFile(filepath.Join(dataDir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	out, errOut, err := runWithStdin(t, project, env, strings.Repeat("y\n", 10), "uninstall", "--purge")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s\n%s", err, out, errOut)
	}
	if strings.Contains(out, "~/.local/bin/termino") {
		t.Errorf("uninstall printed a hardcoded binary path:\n%s", out)
	}
	if !strings.Contains(out, binaryPath) {
		t.Errorf("uninstall should print the actual binary path %s:\n%s", binaryPath, out)
	}
	for _, name := range []string{"termino.db", "termino.db-shm", "termino.db-wal"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s survived --purge", name)
		}
	}
}

func TestDisconnectRemovesEmptySteeringDir(t *testing.T) {
	_, _, project, env := kiroEnv(t)
	mustRun(t, project, env, "connect", "kiro")
	steering := filepath.Join(project, ".kiro", "steering")
	if _, err := os.Stat(filepath.Join(steering, "termino.md")); err != nil {
		t.Fatalf("connect did not write steering: %v", err)
	}
	mustRun(t, project, env, "disconnect", "kiro")
	if _, err := os.Stat(steering); !os.IsNotExist(err) {
		t.Errorf("empty steering dir left behind: %v", err)
	}

	// A steering dir holding other files is kept.
	mustRun(t, project, env, "connect", "kiro")
	if err := os.WriteFile(filepath.Join(steering, "mine.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, project, env, "disconnect", "kiro")
	if _, err := os.Stat(filepath.Join(steering, "mine.md")); err != nil {
		t.Errorf("user steering file removed: %v", err)
	}
}
