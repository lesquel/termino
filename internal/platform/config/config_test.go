package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lesquel/termino/internal/platform/config"
)

// setXDGEnv overrides XDG env vars to point at a temp directory, and returns
// a cleanup function that restores the original values.
func setXDGEnv(t *testing.T, base string) func() {
	t.Helper()
	vars := map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(base, "config"),
		"XDG_DATA_HOME":   filepath.Join(base, "data"),
		"XDG_STATE_HOME":  filepath.Join(base, "state"),
	}
	original := make(map[string]string, len(vars))
	for k := range vars {
		original[k] = os.Getenv(k)
	}
	for k, v := range vars {
		os.Setenv(k, v)
	}
	return func() {
		for k, v := range original {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	tmp := t.TempDir()
	restore := setXDGEnv(t, tmp)
	defer restore()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Theme != "default" {
		t.Errorf("Theme = %q; want %q", cfg.Theme, "default")
	}
	if !cfg.Motion {
		t.Errorf("Motion = false; want true")
	}
}

func TestConfigSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	restore := setXDGEnv(t, tmp)
	defer restore()

	original := &config.Config{
		UserName:         "alice",
		CurrentProjectID: "proj-123",
		Theme:            "spooky",
		Motion:           false,
	}

	if err := config.Save(original); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}

	if loaded.UserName != original.UserName {
		t.Errorf("UserName = %q; want %q", loaded.UserName, original.UserName)
	}
	if loaded.CurrentProjectID != original.CurrentProjectID {
		t.Errorf("CurrentProjectID = %q; want %q", loaded.CurrentProjectID, original.CurrentProjectID)
	}
	if loaded.Theme != original.Theme {
		t.Errorf("Theme = %q; want %q", loaded.Theme, original.Theme)
	}
	if loaded.Motion != original.Motion {
		t.Errorf("Motion = %v; want %v", loaded.Motion, original.Motion)
	}
}

func TestConfigDirPermissions(t *testing.T) {
	tmp := t.TempDir()
	restore := setXDGEnv(t, tmp)
	defer restore()

	cfg := &config.Config{Theme: "default", Motion: true}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Config directory must be 0700.
	info, err := os.Stat(config.ConfigDir())
	if err != nil {
		t.Fatalf("Stat ConfigDir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("ConfigDir perm = %o; want 700", perm)
	}

	// Config file must be 0600.
	filePath := filepath.Join(config.ConfigDir(), "config.toml")
	info, err = os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat config.toml: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config.toml perm = %o; want 600", perm)
	}
}

func TestXDGPathResolution(t *testing.T) {
	tmp := t.TempDir()
	restore := setXDGEnv(t, tmp)
	defer restore()

	if got, want := config.ConfigDir(), filepath.Join(tmp, "config", "termino"); got != want {
		t.Errorf("ConfigDir() = %q; want %q", got, want)
	}
	if got, want := config.DataDir(), filepath.Join(tmp, "data", "termino"); got != want {
		t.Errorf("DataDir() = %q; want %q", got, want)
	}
	if got, want := config.StateDir(), filepath.Join(tmp, "state", "termino"); got != want {
		t.Errorf("StateDir() = %q; want %q", got, want)
	}
}
