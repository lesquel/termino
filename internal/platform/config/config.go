// Package config handles XDG-compliant path resolution and application
// configuration stored as TOML in $XDG_CONFIG_HOME/termino/config.toml.
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all user-level configuration for Termino.
type Config struct {
	UserName         string `toml:"user_name"`
	CurrentProjectID string `toml:"current_project_id"`
	Theme            string `toml:"theme"`
	Motion           bool   `toml:"motion"`
}

// defaults returns a Config with sensible zero-state defaults.
func defaults() Config {
	return Config{
		Theme:  "default",
		Motion: true,
	}
}

// ConfigDir returns $XDG_CONFIG_HOME/termino, falling back to ~/.config/termino.
func ConfigDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "termino")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "termino")
}

// DataDir returns $XDG_DATA_HOME/termino, falling back to ~/.local/share/termino.
func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "termino")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "termino")
}

// StateDir returns $XDG_STATE_HOME/termino, falling back to ~/.local/state/termino.
func StateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "termino")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "termino")
}

// configFilePath returns the full path to config.toml.
func configFilePath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

// Load reads config.toml from the XDG config directory. If the file does not
// exist, Load returns a Config with defaults and no error.
func Load() (*Config, error) {
	cfg := defaults()
	path := configFilePath()

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes cfg to config.toml in the XDG config directory.
// The directory is created with mode 0700; the file is written with mode 0600.
func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(configFilePath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}
