package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
)

// setupKiroHome creates a fake .kiro directory under a temp home and sets HOME.
// Returns the kiro dir path and a restore function.
func setupKiroHome(t *testing.T, existingConfig bool) (kiroDir string, restore func()) {
	t.Helper()
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	os.Setenv("XDG_CONFIG_HOME", "")

	kiroDir = filepath.Join(tmpHome, ".kiro")
	if err := os.MkdirAll(kiroDir, 0700); err != nil {
		t.Fatalf("mkdir .kiro: %v", err)
	}

	if existingConfig {
		settingsDir := filepath.Join(kiroDir, "settings")
		if err := os.MkdirAll(settingsDir, 0700); err != nil {
			t.Fatalf("mkdir settings: %v", err)
		}
		existingConfig := map[string]any{
			"mcpServers": map[string]any{
				"other-tool": map[string]any{
					"command": "other",
					"args":    []string{"run"},
				},
			},
		}
		data, _ := json.MarshalIndent(existingConfig, "", "  ")
		if err := os.WriteFile(filepath.Join(settingsDir, "mcp.json"), data, 0600); err != nil {
			t.Fatalf("write mcp.json: %v", err)
		}
	}

	return kiroDir, func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}
}

func TestKiroConnector_Agent(t *testing.T) {
	c := New()
	if got := c.Agent(); got != "kiro" {
		t.Errorf("Agent() = %q, want %q", got, "kiro")
	}
}

func TestKiroConnector_Detect(t *testing.T) {
	_, restore := setupKiroHome(t, false)
	defer restore()

	c := New()
	detected, err := c.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !detected {
		t.Error("expected .kiro to be detected")
	}
}

func TestKiroConnector_DetectNotFound(t *testing.T) {
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
	}()

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	os.Setenv("XDG_CONFIG_HOME", "")

	c := New()
	detected, err := c.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if detected {
		t.Error("expected .kiro NOT to be detected")
	}
}

func TestKiroConnector_Connect(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, false)
	defer restore()

	c := New()
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 installed items, got %d", len(items))
	}

	// Verify MCP config was written with termino entry.
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal mcp.json: %v", err)
	}
	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("expected mcpServers key in config")
	}
	termino, ok := servers["termino"].(map[string]any)
	if !ok {
		t.Fatal("expected termino entry in mcpServers")
	}
	if cmd, _ := termino["command"].(string); cmd != "termino" {
		t.Errorf("termino.command = %q, want %q", cmd, "termino")
	}
	args, _ := termino["args"].([]any)
	if len(args) != 1 || args[0] != "mcp" {
		t.Errorf("termino.args = %v, want [mcp]", args)
	}

	// Verify steering file created.
	steeringPath := filepath.Join(kiroDir, "steering", "termino.md")
	if _, err := os.Stat(steeringPath); err != nil {
		t.Errorf("steering file not created: %v", err)
	}
}

func TestKiroConnector_ConnectPreservesExistingEntries(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, true)
	defer restore()

	c := New()
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 installed items, got %d", len(items))
	}

	// Verify the existing "other-tool" entry is preserved.
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other-tool"]; !ok {
		t.Error("existing other-tool entry was removed")
	}
	if _, ok := servers["termino"]; !ok {
		t.Error("termino entry was not added")
	}
}

func TestKiroConnector_ConnectIdempotent(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, false)
	defer restore()

	c := New()
	if _, err := c.Connect(connectordomain.ScopeUser); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	// Second connect should not duplicate.
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	// Both artifacts are still reported so the registry re-records them;
	// the MCP entry itself is not written twice (checked below).
	if len(items) != 2 {
		t.Errorf("expected 2 items on second connect (re-registration), got %d", len(items))
	}

	// Verify only one termino entry.
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	data, _ := os.ReadFile(mcpPath)
	var cfg map[string]any
	json.Unmarshal(data, &cfg)
	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["termino"]; !ok {
		t.Error("termino entry missing after second connect")
	}
}

func TestKiroConnector_Disconnect(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, false)
	defer restore()

	c := New()
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Disconnect using the items from Connect.
	if err := c.Disconnect(items); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	// MCP config should no longer have termino entry.
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	var cfg map[string]any
	json.Unmarshal(data, &cfg)
	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["termino"]; ok {
		t.Error("termino entry still present after disconnect")
	}

	// Steering file should be deleted.
	steeringPath := filepath.Join(kiroDir, "steering", "termino.md")
	if _, err := os.Stat(steeringPath); !os.IsNotExist(err) {
		t.Errorf("steering file still exists after disconnect: %v", err)
	}
}

func TestKiroConnector_DisconnectPreservesOtherEntries(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, true)
	defer restore()

	c := New()
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := c.Disconnect(items); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	// other-tool should still be there.
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	data, _ := os.ReadFile(mcpPath)
	var cfg map[string]any
	json.Unmarshal(data, &cfg)
	servers, _ := cfg["mcpServers"].(map[string]any)
	if _, ok := servers["other-tool"]; !ok {
		t.Error("other-tool entry was removed by disconnect")
	}
}

func TestKiroConnector_BackupCreated(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, true)
	defer restore()

	c := New()
	if _, err := c.Connect(connectordomain.ScopeUser); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Backup file should exist.
	backupPath := filepath.Join(kiroDir, "settings", "mcp.json.termino-bak")
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file not created: %v", err)
	}
}
