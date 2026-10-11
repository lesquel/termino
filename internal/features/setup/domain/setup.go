// Package domain defines the setup installation registry and repository
// interface. No imports outside the Go standard library are used.
package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// InstalledItem records a single artifact placed by a Connector.
// This mirrors connector/domain.InstalledItem; it is redeclared here so the
// setup domain does not import the connector domain package.
type InstalledItem struct {
	Kind       string // "file" | "block" | "dir"
	Path       string
	Marker     string
	BackupPath string
}

// InstallationRegistry is a file-backed record of all installed items.
// It is stored at $XDG_STATE_HOME/termino/installed.json with mode 0600.
type InstallationRegistry struct {
	path  string
	Items []InstalledItem
}

// NewInstallationRegistry creates a registry that will persist at the given
// path. Call Load() to read an existing registry.
func NewInstallationRegistry(path string) *InstallationRegistry {
	return &InstallationRegistry{path: path}
}

// Load reads the registry file from disk. If the file does not exist, the
// registry starts empty (no error).
func (r *InstallationRegistry) Load() error {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		r.Items = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("read registry: %w", err)
	}
	if err := json.Unmarshal(data, &r.Items); err != nil {
		return fmt.Errorf("parse registry: %w", err)
	}
	return nil
}

// Save writes the registry to disk with mode 0600.
func (r *InstallationRegistry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	data, err := json.MarshalIndent(r.Items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(r.path, data, 0600); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

// Record appends an item to the registry and persists the change.
func (r *InstallationRegistry) Record(item InstalledItem) error {
	r.Items = append(r.Items, item)
	return r.Save()
}

// Remove removes the first item matching Path and Marker and persists the change.
func (r *InstallationRegistry) Remove(item InstalledItem) error {
	filtered := r.Items[:0]
	for _, existing := range r.Items {
		if existing.Path == item.Path && existing.Marker == item.Marker {
			continue
		}
		filtered = append(filtered, existing)
	}
	r.Items = filtered
	return r.Save()
}

// List returns all recorded items.
func (r *InstallationRegistry) List() ([]InstalledItem, error) {
	return r.Items, nil
}

// SetupRepository provides access to the installation registry.
type SetupRepository interface {
	// GetRegistry returns the loaded installation registry.
	GetRegistry() *InstallationRegistry
}
