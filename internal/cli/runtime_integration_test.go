package cli_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// isolatedEnv returns XDG and HOME overrides rooted at a fresh temp dir.
func isolatedEnv(t *testing.T) (string, []string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	return tmp, []string{
		"HOME=" + home,
		"SHELL=/bin/bash",
		"XDG_DATA_HOME=" + filepath.Join(tmp, "data"),
		"XDG_CONFIG_HOME=" + filepath.Join(tmp, "config"),
		"XDG_STATE_HOME=" + filepath.Join(tmp, "state"),
	}
}

// runIn executes the binary with dir as its working directory.
func runIn(t *testing.T, dir string, env []string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// TRD 3: logs go to a file, never to stdout — for the MCP server stdout is the
// protocol channel, so every stdout line must be a JSON-RPC message.
func TestMCPLogsToFileAndKeepsStdoutProtocolOnly(t *testing.T) {
	tmp, env := isolatedEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "mcp")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}` + "\n")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // stdin EOF ends the server; exit status is not the point

	lines := 0
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines++
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg["jsonrpc"] != "2.0" {
			t.Fatalf("non-protocol line on stdout: %q", line)
		}
	}
	if lines == 0 {
		t.Fatalf("no initialize response on stdout (stderr: %s)", stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(tmp, "state", "termino", "termino.log"))
	if err != nil {
		t.Fatalf("mcp did not create the log file: %v", err)
	}
	if !strings.Contains(string(data), "MCP server starting") {
		t.Fatalf("log file lacks the server start record: %s", data)
	}
}

// `termino disconnect kiro` must remove only Kiro's registry items; the shell
// widget block installed separately stays installed.
func TestDisconnectRemovesOnlyThatAgentsItems(t *testing.T) {
	tmp, env := isolatedEnv(t)
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(filepath.Join(home, ".kiro"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(tmp, "project")
	_ = os.MkdirAll(project, 0o700)

	for _, args := range [][]string{{"widget", "install"}, {"connect", "kiro"}, {"disconnect", "kiro"}} {
		if out, errOut, err := runIn(t, project, env, args...); err != nil {
			t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", args, err, out, errOut)
		}
	}

	reg, err := os.ReadFile(filepath.Join(tmp, "state", "termino", "installed.json"))
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if !strings.Contains(string(reg), "termino-widget-start") {
		t.Errorf("widget item dropped from registry by disconnect: %s", reg)
	}
	if strings.Contains(string(reg), "termino-steering") {
		t.Errorf("kiro item still in registry after disconnect: %s", reg)
	}
	rc, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if !strings.Contains(string(rc), "termino-widget-start") {
		t.Errorf("widget block removed from .bashrc: %s", rc)
	}
}

// `termino import <file> --watch` keeps running until interrupted and then
// stops cleanly.
func TestImportWatchStopsOnInterrupt(t *testing.T) {
	tmp, env := isolatedEnv(t)
	if out, errOut, err := runIn(t, tmp, env, "project", "new", "--name", "W", "--key", "WAT", "--methodology", "kanban"); err != nil {
		t.Fatalf("project new: %v\n%s%s", err, out, errOut)
	}
	if err := os.WriteFile(filepath.Join(tmp, "tasks.md"), []byte("- [ ] one\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "import", "tasks.md", "--watch")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), env...)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(pipe)
	var seen []string
	watching := false
	for sc.Scan() {
		seen = append(seen, sc.Text())
		if strings.HasPrefix(sc.Text(), "Watching") {
			watching = true
			break
		}
	}
	if !watching {
		_ = cmd.Process.Kill()
		t.Fatalf("watcher never started; stdout: %v", seen)
	}
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	for sc.Scan() {
		seen = append(seen, sc.Text())
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("import --watch exited with %v; stdout: %v", err, seen)
	}
	if !strings.Contains(strings.Join(seen, "\n"), "Watcher stopped") {
		t.Fatalf("missing clean stop message; stdout: %v", seen)
	}
}
