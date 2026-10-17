// Package kiro provides a Connector implementation for the Kiro AI editor.
// It writes a termino MCP entry into ~/.kiro/settings/mcp.json (user scope)
// or <project>/.kiro/settings/mcp.json (project scope), and the steering
// file <project>/.kiro/steering/termino.md (TRD 10.3). Without a project
// root the steering file falls back to Kiro's global ~/.kiro/steering/.
//
// Paths follow Kiro's documented layout: workspace config lives under the
// project's .kiro/ directory and user config under ~/.kiro/.
package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
)

const (
	mcpEntryKey     = "termino"
	steeringFile    = "termino.md"
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

// steeringMarker identifies the steering file in the installation registry.
const steeringMarker = "termino-steering"

// Connector implements connectordomain.Connector for Kiro.
type Connector struct {
	projectRoot string
}

// New returns a Kiro Connector.
func New() *Connector { return &Connector{} }

// WithProjectRoot sets the project whose .kiro/steering/ receives the steering
// file (and whose .kiro/settings/mcp.json is used for project scope).
func (c *Connector) WithProjectRoot(root string) *Connector {
	c.projectRoot = root
	return c
}

// Owns reports whether item was installed by this connector, so disconnect
// touches only Kiro's registry entries.
func (c *Connector) Owns(item connectordomain.InstalledItem) bool {
	switch item.Marker {
	case steeringMarker:
		return true
	case mcpEntryKey:
		return strings.Contains(filepath.ToSlash(item.Path), "/.kiro/")
	default:
		return false
	}
}

// Describe phrases what removing item does: the MCP config is shared, so only
// the termino entry leaves it; the steering file is deleted.
func (c *Connector) Describe(item connectordomain.InstalledItem) string {
	if item.Marker == mcpEntryKey {
		return "termino entry from " + filepath.Base(item.Path)
	}
	return "file " + filepath.Base(item.Path)
}

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
	mcpDir := kiroDir
	if scope == connectordomain.ScopeProject && c.projectRoot != "" {
		mcpDir = filepath.Join(c.projectRoot, ".kiro")
	}
	steeringBase := kiroDir
	if c.projectRoot != "" {
		steeringBase = filepath.Join(c.projectRoot, ".kiro")
	}

	var installed []connectordomain.InstalledItem

	// ── MCP config ───────────────────────────────────────────────────────────
	mcpPath := filepath.Join(mcpDir, "settings", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0700); err != nil {
		return nil, fmt.Errorf("create mcp settings dir: %w", err)
	}

	// Read existing config (may not exist yet). An unexpected format aborts
	// before anything is written.
	doc, servers, err := readMCPConfig(mcpPath)
	if err != nil {
		return nil, err
	}

	// Only add if not already present; an existing entry is still reported
	// so the registry re-records it.
	backupPath := mcpPath + ".termino-bak"
	if _, ok := servers[mcpEntryKey]; !ok {
		// Back up before modifying.
		if err := backupFile(mcpPath, backupPath); err != nil {
			return nil, fmt.Errorf("backup mcp config: %w", err)
		}
		entry, err := json.Marshal(&mcpServerEntry{
			Command: "termino",
			Args:    []string{"mcp"},
			Env:     map[string]string{},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal mcp entry: %w", err)
		}
		servers[mcpEntryKey] = entry
		if err := writeMCPConfig(mcpPath, doc, servers); err != nil {
			return nil, err
		}
	}
	mcpItem := connectordomain.InstalledItem{
		Kind:   connectordomain.ItemKindFile,
		Path:   mcpPath,
		Marker: mcpEntryKey,
	}
	if _, err := os.Stat(backupPath); err == nil {
		mcpItem.BackupPath = backupPath
	}
	installed = append(installed, mcpItem)

	// ── Steering file ─────────────────────────────────────────────────────────
	steeringDir := filepath.Join(steeringBase, "steering")
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
		Marker: steeringMarker,
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
			// The entry is gone cleanly, so the pre-install backup is stale.
			backup := item.BackupPath
			if backup == "" {
				backup = item.Path + ".termino-bak"
			}
			if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove mcp config backup: %w", err)
			}
		case item.Marker == steeringMarker:
			if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove steering file: %w", err)
			}
			// Drop the steering dir too when nothing else lives in it;
			// os.Remove refuses a non-empty directory.
			_ = os.Remove(filepath.Dir(item.Path))
		default:
			// Unknown item — skip.
		}
	}
	return nil
}

// removeMCPEntry removes the "termino" key from the MCP config at path,
// leaving every other key untouched.
func (c *Connector) removeMCPEntry(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	doc, servers, err := readMCPConfig(path)
	if err != nil {
		return err
	}
	if _, ok := servers[mcpEntryKey]; !ok {
		return nil
	}
	delete(servers, mcpEntryKey)
	return writeMCPConfig(path, doc, servers)
}

// kiroDir returns the path to the .kiro directory.
func (c *Connector) kiroDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kiro dir: home dir: %w", err)
	}
	return filepath.Join(home, ".kiro"), nil
}

// readMCPConfig reads Kiro's MCP config as raw JSON so unknown keys survive
// a rewrite: doc holds every top-level key and servers every entry of
// "mcpServers", each kept verbatim. A missing or empty file yields empty
// maps. Anything that is not a JSON object with an object-valued
// "mcpServers" is an unexpected format and is reported, never overwritten.
func readMCPConfig(path string) (doc, servers map[string]json.RawMessage, err error) {
	doc = make(map[string]json.RawMessage)
	servers = make(map[string]json.RawMessage)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return doc, servers, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read mcp config: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return doc, servers, nil
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		return nil, nil, fmt.Errorf("unexpected format in %s (want a JSON object) — add the termino server to mcpServers by hand: %v", path, err)
	}
	if raw, ok := doc["mcpServers"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, nil, fmt.Errorf("unexpected format in %s (mcpServers is not an object) — add the termino server by hand: %v", path, err)
		}
		if servers == nil {
			servers = make(map[string]json.RawMessage)
		}
	}
	return doc, servers, nil
}

// writeMCPConfig stores servers under "mcpServers" in doc and writes doc to
// path, preserving every other top-level key.
func writeMCPConfig(path string, doc, servers map[string]json.RawMessage) error {
	rawServers, err := json.Marshal(servers)
	if err != nil {
		return fmt.Errorf("marshal mcp servers: %w", err)
	}
	doc["mcpServers"] = rawServers
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
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
