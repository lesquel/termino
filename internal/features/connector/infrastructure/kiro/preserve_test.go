package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
)

// TRD 10.3 rule 1: a connector only adds or removes its own entry and never
// rewrites the rest of the file — unknown top-level keys and unknown fields
// of other servers must survive connect and disconnect.
func TestKiroConnector_PreservesUnknownKeys(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, false)
	defer restore()
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o700); err != nil {
		t.Fatal(err)
	}
	original := `{
  "powers": {"enabled": true},
  "mcpServers": {
    "other-tool": {"command": "other", "args": ["run"], "disabled": true, "autoApprove": ["read"]}
  }
}`
	if err := os.WriteFile(mcpPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	c := New()
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	assertPreserved(t, mcpPath, true)

	if err := c.Disconnect(items); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	assertPreserved(t, mcpPath, false)
}

func assertPreserved(t *testing.T, path string, wantTermino bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg struct {
		Powers     map[string]any            `json:"powers"`
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", data, err)
	}
	if cfg.Powers["enabled"] != true {
		t.Errorf("top-level key lost: %s", data)
	}
	other := cfg.MCPServers["other-tool"]
	if other["disabled"] != true || other["autoApprove"] == nil {
		t.Errorf("unknown fields of other-tool lost: %s", data)
	}
	if _, ok := cfg.MCPServers["termino"]; ok != wantTermino {
		t.Errorf("termino entry present = %v, want %v: %s", ok, wantTermino, data)
	}
}

// A file whose mcpServers is not an object is not the expected format: write
// nothing (TRD 10.3 rule 3).
func TestKiroConnector_RefusesUnexpectedFormat(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, false)
	defer restore()
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	_ = os.MkdirAll(filepath.Dir(mcpPath), 0o700)
	original := `{"mcpServers": ["not", "an", "object"]}`
	_ = os.WriteFile(mcpPath, []byte(original), 0o600)

	if _, err := New().Connect(connectordomain.ScopeUser); err == nil {
		t.Fatal("connect accepted an unexpected mcp.json format")
	}
	data, _ := os.ReadFile(mcpPath)
	if string(data) != original {
		t.Fatalf("mcp.json was modified: %s", data)
	}
}

// TRD 10.3: the steering file belongs in the project's .kiro/steering/.
func TestKiroConnector_SteeringInProjectRoot(t *testing.T) {
	kiroDir, restore := setupKiroHome(t, false)
	defer restore()
	project := t.TempDir()

	c := New().WithProjectRoot(project)
	items, err := c.Connect(connectordomain.ScopeUser)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	want := filepath.Join(project, ".kiro", "steering", "termino.md")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("project steering file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(kiroDir, "steering", "termino.md")); !os.IsNotExist(err) {
		t.Fatalf("steering also written under the home .kiro: %v", err)
	}
	if _, err := os.Stat(filepath.Join(kiroDir, "settings", "mcp.json")); err != nil {
		t.Fatalf("user-scope MCP config missing: %v", err)
	}
	if err := c.Disconnect(items); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("steering file survived disconnect: %v", err)
	}
}

func TestKiroConnector_OwnsOnlyItsItems(t *testing.T) {
	c := New()
	cases := []struct {
		item connectordomain.InstalledItem
		want bool
	}{
		{connectordomain.InstalledItem{Kind: "file", Path: "/h/.kiro/settings/mcp.json", Marker: "termino"}, true},
		{connectordomain.InstalledItem{Kind: "file", Path: "/p/.kiro/steering/termino.md", Marker: "termino-steering"}, true},
		{connectordomain.InstalledItem{Kind: "block", Path: "/h/.bashrc", Marker: "termino-widget-start"}, false},
		{connectordomain.InstalledItem{Kind: "file", Path: "/h/.other-agent.json", Marker: "termino"}, false},
	}
	for _, tc := range cases {
		if got := c.Owns(tc.item); got != tc.want {
			t.Errorf("Owns(%+v) = %v, want %v", tc.item, got, tc.want)
		}
	}
}
