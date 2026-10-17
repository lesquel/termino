package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesquel/termino/internal/platform/sqlite"
)

// Activity from the CLI is attributed to the setup user's member, not "cli".
func TestActivityIsAttributedToSetupUser(t *testing.T) {
	tmp, env := isolatedEnv(t)
	answers := "Leo\nDemo\nDEM\n\nn\nspooky\nn\n"
	if out, errOut, err := runWithStdin(t, tmp, env, answers, "setup"); err != nil {
		t.Fatalf("setup: %v\n%s\n%s", err, out, errOut)
	}
	mustRun(t, tmp, env, "add", "Mine")

	db, err := sqlite.OpenReadOnly(filepath.Join(tmp, "data", "termino", "termino.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT actor_id FROM activity`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var actor string
		if err := rows.Scan(&actor); err != nil {
			t.Fatal(err)
		}
		n++
		if actor != "Leo" {
			t.Errorf("activity actor = %q, want the setup user Leo", actor)
		}
	}
	if n == 0 {
		t.Fatal("no activity recorded")
	}
}

func TestUninstallWithNothingRemovedSaysSo(t *testing.T) {
	_, _, project, env := kiroEnv(t)
	out, errOut, err := runWithStdin(t, project, env, "", "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, errOut)
	}
	if strings.Contains(out, "Uninstall complete") || !strings.Contains(out, "Nothing removed.") {
		t.Errorf("empty uninstall should say Nothing removed:\n%s", out)
	}

	mustRun(t, project, env, "connect", "kiro")
	out, _, _ = runWithStdin(t, project, env, strings.Repeat("n\n", 10), "uninstall")
	if strings.Contains(out, "Uninstall complete") || !strings.Contains(out, "Nothing removed.") {
		t.Errorf("fully declined uninstall should say Nothing removed:\n%s", out)
	}
	if !strings.Contains(out, "Remove termino entry from mcp.json") {
		t.Errorf("mcp.json prompt should offer removing the termino entry:\n%s", out)
	}
	if strings.Contains(out, "Remove file (") && strings.Contains(lineWith(out, "Remove file ("), "mcp.json") {
		t.Errorf("mcp.json prompt should not offer removing the file:\n%s", out)
	}
}

func TestUninstallRemovesMCPBackup(t *testing.T) {
	_, home, project, env := kiroEnv(t)
	mcpPath := filepath.Join(home, ".kiro", "settings", "mcp.json")
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"other":{"command":"x","args":[]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, project, env, "connect", "kiro")
	if _, err := os.Stat(mcpPath + ".termino-bak"); err != nil {
		t.Fatalf("connect should back up mcp.json: %v", err)
	}
	out, errOut, err := runWithStdin(t, project, env, strings.Repeat("y\n", 10), "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v\n%s\n%s", err, out, errOut)
	}
	if _, err := os.Stat(mcpPath + ".termino-bak"); !os.IsNotExist(err) {
		t.Errorf("mcp.json backup left behind after a clean removal (err=%v)", err)
	}
	if !strings.Contains(out, "Uninstall complete.") {
		t.Errorf("uninstall that removed items should say Uninstall complete:\n%s", out)
	}
}
