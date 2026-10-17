package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	apptui "github.com/lesquel/termino/internal/app/tui"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
	"github.com/lesquel/termino/internal/platform/config"
)

type rootProjectRepo struct{ projects []projectdomain.Project }

func (r rootProjectRepo) Save(context.Context, projectdomain.Project) error { return nil }
func (r rootProjectRepo) FindByID(_ context.Context, id string) (projectdomain.Project, error) {
	for _, p := range r.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return projectdomain.Project{}, errors.New("not found")
}
func (r rootProjectRepo) FindByKey(context.Context, string) (projectdomain.Project, error) {
	return projectdomain.Project{}, errors.New("not found")
}
func (r rootProjectRepo) List(context.Context) ([]projectdomain.Project, error) {
	return r.projects, nil
}
func (r rootProjectRepo) Archive(context.Context, string, time.Time) error { return nil }

func newProjectRoot(t *testing.T) tea.Model {
	t.Helper()
	cols, tasks := screenFixture()
	theme, _ := apptui.Load("default")
	repo := rootBoardRepo{cols: cols}
	deps := apptui.RootDeps{
		Config:    &config.Config{CurrentProjectID: "p1"},
		BoardUC:   boardapp.NewBoardUseCases(repo, rootTaskRepo{tasks: tasks}),
		ProjectUC: projectapp.NewProjectUseCases(rootProjectRepo{projects: []projectdomain.Project{{ID: "p1", Name: "Web App", Key: "WEB"}}}, repo, nil),
		Theme:     theme,
	}
	var m tea.Model = apptui.NewRootModel(deps)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return loadBoardInto(t, m, deps)
}

func TestRootBoardHeaderNamesTheProject(t *testing.T) {
	view := ansi.Strip(newProjectRoot(t).View())
	if !strings.Contains(strings.Split(view, "\n")[0], "termino · Web App (WEB)") {
		t.Errorf("expected project header, got:\n%s", view)
	}
	if !strings.Contains(view, "WEB-1 Hello") {
		t.Errorf("expected KEY-N card ref, got:\n%s", view)
	}
}

func TestRootTaskCreatedShowsFeedbackOnBoard(t *testing.T) {
	m := newProjectRoot(t)
	m, _ = m.Update(tasktui.TaskCreatedMsg{Task: taskdomain.Task{ID: "t2", Number: 2, Title: "Fresh"}})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Created WEB-2 Fresh") {
		t.Errorf("expected creation feedback on the board, got:\n%s", view)
	}
}

// TestRootFormCapturesGlobalKeys proves that q, p and ? are typed into the
// new-task form instead of quitting, opening the selector or the help.
func TestRootFormCapturesGlobalKeys(t *testing.T) {
	m := newProjectRoot(t)
	m, _ = m.Update(key("n"))
	for _, k := range []string{"q", "p", "?"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		if isQuit(cmd) {
			t.Fatalf("%q quit the program from the form", k)
		}
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "New Task") || !strings.Contains(view, "qp?") {
		t.Errorf("expected the keys typed into the form title, got:\n%s", view)
	}
}

func TestRootDetailEditCapturesGlobalKeys(t *testing.T) {
	m := newProjectRoot(t)
	m, _ = m.Update(key("enter"))
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{ID: "t1", Number: 1, Title: "Hello", ColumnID: "c1"}})
	m, _ = m.Update(key("e"))
	for _, k := range []string{"q", "p"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		if isQuit(cmd) {
			t.Fatalf("%q quit the program while editing", k)
		}
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Edit Task") || !strings.Contains(view, "Helloqp") {
		t.Errorf("expected keys typed into the title, got:\n%s", view)
	}
}
