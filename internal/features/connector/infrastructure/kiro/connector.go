// Package kiro provides a Connector implementation for the Kiro AI editor.
// It writes a termino MCP entry into ~/.kiro/settings/mcp.json and
// a steering file at ~/.kiro/steering/termino.md.
//
// TODO: verify mcp.json path against current Kiro documentation — the path
// used here is ~/.kiro/settings/mcp.json which was correct at implementation time.
package kiro

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
)

const (
	mcpEntryKey    = "termino"
	steeringFile   = "termino.md"
	steeringContent = `# Termino MCP Integration

Termino MCP is available. Use the following tools to track work:

- **list_projects** — list all projects
- **get_board** — view the Kanban board for a project
- **claim_task** — assign a task to yourself and move it to In Progress
- **complete_task** — move a task to the Done column
- **add_comment** — add a comment to a task

Always call **claim_task** when you start working on a task so the board reflects
the current state of work.
`
)

// mcpServerEntry is the shape written into Kiro's mcp.json.
type mcpServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Connector implements connectordomain.Connector for Kiro.
type Connector struct{}

// New returns a Kiro Connector.
func New() *Connector { return &Connector{} }

// Agent returns the agent identifier.
func (c *Connector) Agent() string { return "kiro" }

// Detect returns true if a .kiro directory exists at $HOME or $XDG_CONFIG_HOME/kiro.
func (c *Connector) Detect() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("kiro.Detect home dir: %w", err)
	}

	candidates := []string{
		filepath.Join(home, ".kiro"),
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "kiro"))
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

// Connect installs the termino MCP entry and the steering file.
// It backs up the MCP config before modifying it.
// Returns the list of items installed (for registry recording).
func (c *Connector) Connect(scope connectordomain.Scope) ([]connectordomain.InstalledItem, error) {
	kiroDir, err := c.kiroDir()
	if err != nil {
		return nil, err
	}

	var installed []connectordomain.InstalledItem

	// ── MCP config ───────────────────────────────────────────────────────────
	mcpPath := filepath.Join(kiroDir, "settings", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0700); err != nil {
		return nil, fmt.Errorf("create mcp settings dir: %w", err)
	}

	// Read existing config (may not exist yet).
	mcpConfig, err := readMCPConfig(mcpPath)
	if err != nil {
		return nil, err
	}

	// Back up before modifying.
	backupPath := mcpPath + ".termino-bak"
	if err := backupFile(mcpPath, backupPath); err != nil {
		return nil, fmt.Errorf("backup mcp config: %w", err)
	}

	// Only add if not already present.
	added := false
	if mcpConfig[mcpEntryKey] == nil {
		mcpConfig[mcpEntryKey] = &mcpServerEntry{
			Command: "termino",
			Args:    []string{"mcp"},
			Env:     map[string]string{},
		}
		added = true
	}

	if added {
		if err := writeMCPConfig(mcpPath, mcpConfig); err != nil {
			return nil, err
		}
		installed = append(installed, connectordomain.InstalledItem{
			Kind:       connectordomain.ItemKindFile,
			Path:       mcpPath,
			Marker:     mcpEntryKey,
			BackupPath: backupPath,
		})
	}

	// ── Steering file ─────────────────────────────────────────────────────────
	steeringDir := filepath.Join(kiroDir, "steering")
	if err := os.MkdirAll(steeringDir, 0700); err != nil {
		return nil, fmt.Errorf("create steering dir: %w", err)
	}
	steeringPath := filepath.Join(steeringDir, steeringFile)
	if err := os.WriteFile(steeringPath, []byte(steeringContent), 0600); err != nil {
		return nil, fmt.Errorf("write steering file: %w", err)
	}
	installed = append(installed, connectordomain.InstalledItem{
		Kind:   connectordomain.ItemKindFile,
		Path:   steeringPath,
		Marker: "termino-steering",
	})

	return installed, nil
}

// Disconnect removes the termino MCP entry and steering file that were
// recorded in the items list.
func (c *Connector) Disconnect(items []connectordomain.InstalledItem) error {
	for _, item := range items {
		switch {
		case item.Marker == mcpEntryKey:
			// Remove the termino key from mcp.json.
			if err := c.removeMCPEntry(item.Path); err != nil {
				return err
			}
		case item.Marker == "termino-steering":
			if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove steering file: %w", err)
			}
		default:
			// Unknown item — skip.
		}
	}
	return nil
}

// removeMCPEntry removes the "termino" key from the MCP config at path.
func (c *Connector) removeMCPEntry(path string) error {
	config, err := readMCPConfig(path)
	if err != nil {
		return err
	}
	delete(config, mcpEntryKey)
	return writeMCPConfig(path, config)
}

// kiroDir returns the path to the .kiro directory.
func (c *Connector) kiroDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kiro dir: home dir: %w", err)
	}
	return filepath.Join(home, ".kiro"), nil
}

// readMCPConfig reads the MCP config JSON, returning an empty map if the
// file does not exist. It handles both the wrapped {"mcpServers": {...}}
// format (which is what writeMCPConfig produces) and a flat top-level map.
func readMCPConfig(path string) (map[string]*mcpServerEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]*mcpServerEntry), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read mcp config: %w", err)
	}

	// Try the wrapped format first (the format writeMCPConfig produces).
	var wrapped struct {
		MCPServers map[string]*mcpServerEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.MCPServers != nil {
		return wrapped.MCPServers, nil
	}

	// Fall back to a flat top-level map.
	var cfg map[string]*mcpServerEntry
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mcp config: %w", err)
	}
	if cfg == nil {
		cfg = make(map[string]*mcpServerEntry)
	}
	return cfg, nil
}

// writeMCPConfig writes the MCP config to path, wrapped in {"mcpServers": ...}.
func writeMCPConfig(path string, cfg map[string]*mcpServerEntry) error {
	wrapped := map[string]any{"mcpServers": cfg}
	data, err := json.MarshalIndent(wrapped, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}
	return nil
}

// backupFile copies src to dst. No-op if src does not exist.
func backupFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}
