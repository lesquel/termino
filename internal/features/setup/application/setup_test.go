package application

import (
	"context"
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

type lineRun struct {
	projects *projectinfra.ProjectRepository
	detector *projectinfra.PathDetector
	uc       *projectapp.ProjectUseCases
	out      *strings.Builder
}

// newLineRun prepares an isolated environment for the line-prompt wizard.
func newLineRun(t *testing.T) lineRun {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	db, err := sqlite.Open(filepath.Join(dir, "termino.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	projects := projectinfra.NewProjectRepository(db)
	return lineRun{
		projects: projects,
		detector: projectinfra.NewPathDetector(db),
		uc:       projectapp.NewProjectUseCases(projects, boardinfra.NewBoardRepository(db), events.NewBus()),
	}
}

// run drives the wizard with stdin answers from cwd and returns its output.
func (r lineRun) run(t *testing.T, cwd, answers string) string {
	t.Helper()
	reg := setupdomain.NewInstallationRegistry(filepath.Join(t.TempDir(), "installed.json"))
	w := NewSetupWizard(r.uc, reg)
	w.in = strings.NewReader(answers)
	out := &strings.Builder{}
	w.out = out
	w.svc.getwd = func() (string, error) { return cwd, nil }
	w.svc.getenv = func(k string) string {
		if k == "SHELL" {
			return "/bin/bash"
		}
		if k == "USER" {
			return "leo"
		}
		return ""
	}
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	return out.String()
}

// Answers: name, project name, key, methodology, associate, theme, widget.
func TestSetupWizard_AssociatesWorkingDirectoryByDefault(t *testing.T) {
	r := newLineRun(t)
	r.run(t, "/work/web", "Ana\nWeb\nWEB\n\n\n\nn\n")
	p, err := r.projects.FindByKey(context.Background(), "WEB")
	if err != nil {
		t.Fatal(err)
	}
	if p.AssociatedPath != "/work/web" {
		t.Fatalf("AssociatedPath = %q, want /work/web", p.AssociatedPath)
	}
	if detected, _ := r.detector.DetectForPath("/work/web/sub"); detected == "" {
		t.Fatal("detector did not resolve the project for a subdirectory of the associated path")
	}
}

func TestSetupWizard_SkipsAssociationWhenDeclined(t *testing.T) {
	r := newLineRun(t)
	r.run(t, "/work/web", "Ana\nWeb\nWEB\n\nn\n\nn\n")
	p, err := r.projects.FindByKey(context.Background(), "WEB")
	if err != nil {
		t.Fatal(err)
	}
	if p.AssociatedPath != "" {
		t.Fatalf("AssociatedPath = %q, want empty", p.AssociatedPath)
	}
}

func TestSetupWizard_DefaultsInBracketsAndRepromptOnInvalidKey(t *testing.T) {
	r := newLineRun(t)
	// Blank name/project accept defaults; key "x1" is rejected, then "web" is
	// upper-cased; methodology scrum; associate default; theme default; no widget.
	out := r.run(t, "/work/web-app", "\n\nx1\nweb\nscrum\n\n\nn\n")
	for _, want := range []string{"Your name [leo]:", "Project name [web-app]:", "Project key (2-5 uppercase letters) [WA]:", "✗ key", "Next steps"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if _, err := r.projects.FindByKey(context.Background(), "WEB"); err != nil {
		t.Fatalf("project WEB not created: %v", err)
	}
}

func TestSetupWizard_RerunEditDoesNotDuplicate(t *testing.T) {
	r := newLineRun(t)
	r.run(t, "/work/web", "Ana\nWeb\nWEB\n\n\n\nn\n")
	// e(dit), name, new project name, methodology, associate, theme, widget.
	out := r.run(t, "/work/web", "e\n\nWeb App\nscrum\n\n\nn\n")
	if !strings.Contains(out, "cannot be changed") || !strings.Contains(out, "Project updated: Web App (WEB)") {
		t.Fatalf("unexpected edit output:\n%s", out)
	}
	list, _ := r.uc.ListProjects(context.Background())
	if len(list) != 1 || list[0].Name != "Web App" {
		t.Fatalf("projects after edit = %+v, want one renamed project", list)
	}
	// Cancel (the default) changes nothing.
	if out := r.run(t, "/work/web", "\n"); !strings.Contains(out, "Nothing was changed") {
		t.Fatalf("cancel output:\n%s", out)
	}
}
