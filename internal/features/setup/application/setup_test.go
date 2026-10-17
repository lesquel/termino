package application

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

// runWizard drives the wizard with the given stdin answers and returns the
// created project's associated path plus the detector's verdict for cwd.
func runWizard(t *testing.T, cwd, answers string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	db, err := sqlite.Open(filepath.Join(dir, "termino.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	projects := projectinfra.NewProjectRepository(db)
	uc := projectapp.NewProjectUseCases(projects, boardinfra.NewBoardRepository(db), events.NewBus())
	reg := setupdomain.NewInstallationRegistry(filepath.Join(dir, "state", "installed.json"))

	w := NewSetupWizard(uc, reg)
	w.in = strings.NewReader(answers)
	w.out = io.Discard
	w.getwd = func() (string, error) { return cwd, nil }

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	p, err := projects.FindByKey(context.Background(), "WEB")
	if err != nil {
		t.Fatalf("find project: %v", err)
	}
	detected, err := projectinfra.NewPathDetector(db).DetectForPath(filepath.Join(cwd, "sub"))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	return p.AssociatedPath, detected
}

func TestSetupWizard_AssociatesWorkingDirectoryByDefault(t *testing.T) {
	// name, project name, key, associate (blank = yes), widget, theme.
	path, detected := runWizard(t, "/work/web", "Ana\nWeb\nWEB\n\nn\n\n")
	if path != "/work/web" {
		t.Fatalf("AssociatedPath = %q, want /work/web", path)
	}
	if detected == "" {
		t.Fatal("detector did not resolve the project for a subdirectory of the associated path")
	}
}

func TestSetupWizard_SkipsAssociationWhenDeclined(t *testing.T) {
	path, detected := runWizard(t, "/work/web", "Ana\nWeb\nWEB\nn\nn\n\n")
	if path != "" || detected != "" {
		t.Fatalf("AssociatedPath = %q, detected = %q; want both empty", path, detected)
	}
}
