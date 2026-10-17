package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
)

func newShellWidget(t *testing.T, shell string) (*ShellWidget, string, string) {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(dir, "state", "installed.json")
	return NewShellWidget(setupdomain.NewInstallationRegistry(regPath), shell, home), home, regPath
}

func TestShellWidgetInstallAndUninstall(t *testing.T) {
	w, home, _ := newShellWidget(t, "/bin/zsh")
	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte("export A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := w.Status()
	if err != nil || st.Installed || st.RCPath != rc || st.Shell.Name != "zsh" {
		t.Fatalf("initial status = %+v, %v", st, err)
	}

	path, err := w.Install()
	if err != nil || path != rc {
		t.Fatalf("Install = %q, %v", path, err)
	}
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), WidgetMarker) || !strings.HasPrefix(string(data), "export A=1\n") {
		t.Fatalf("rc after install:\n%s", data)
	}
	// Installing twice keeps a single block.
	if _, err := w.Install(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(rc)
	if n := strings.Count(string(data), WidgetMarker); n != 1 {
		t.Fatalf("%d widget blocks after a second install", n)
	}
	if st, _ := w.Status(); !st.Installed || st.InstalledPath != rc {
		t.Fatalf("status after install = %+v", st)
	}

	path, err = w.Uninstall()
	if err != nil || path != rc {
		t.Fatalf("Uninstall = %q, %v", path, err)
	}
	data, _ = os.ReadFile(rc)
	if string(data) != "export A=1\n" {
		t.Fatalf("rc after uninstall = %q", data)
	}
	if st, _ := w.Status(); st.Installed {
		t.Fatalf("still installed after uninstall: %+v", st)
	}
}

func TestShellWidgetFishIsNotSupported(t *testing.T) {
	w, home, _ := newShellWidget(t, "/usr/bin/fish")
	st, _ := w.Status()
	if st.Shell.Supported || st.RCPath != "" {
		t.Fatalf("fish status = %+v", st)
	}
	if _, err := w.Install(); err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("Install on fish = %v", err)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("fish install wrote files: %v", entries)
	}
}

// Another termino process may install the widget; Status sees it.
func TestShellWidgetStatusReloadsRegistry(t *testing.T) {
	w, home, regPath := newShellWidget(t, "/bin/bash")
	other := setupdomain.NewInstallationRegistry(regPath)
	if _, err := InstallWidget(other, filepath.Join(home, ".bashrc")); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.Status(); !st.Installed {
		t.Fatal("status did not see an install from another registry handle")
	}
}
