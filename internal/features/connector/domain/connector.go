// Package domain defines the connector strategy interface used to integrate
// external AI agents (Kiro, Claude Code, Codex, OpenCode) into the workspace.
// No imports outside the Go standard library are used.
package domain

// ItemKind classifies an installed artifact.
type ItemKind string

const (
	ItemKindFile  ItemKind = "file"
	ItemKindBlock ItemKind = "block"
	ItemKindDir   ItemKind = "dir"
)

// Scope determines whether the connector operates at the project or user level.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

// InstalledItem records a single artifact placed by a Connector.
type InstalledItem struct {
	Kind       ItemKind
	Path       string
	Marker     string
	BackupPath string
}

// Connector is the strategy interface implemented by each agent adapter.
type Connector interface {
	// Agent returns the identifier for the target agent (e.g. "kiro").
	Agent() string
	// Detect reports whether the agent is available in the current environment.
	Detect() (bool, error)
	// Connect installs the termino integration for the given scope and returns
	// the list of items that were installed.
	Connect(scope Scope) ([]InstalledItem, error)
	// Disconnect removes all previously installed items.
	Disconnect(items []InstalledItem) error
	// Owns reports whether item was installed by this connector, so that
	// disconnecting one agent never touches another integration's items.
	Owns(item InstalledItem) bool
}
