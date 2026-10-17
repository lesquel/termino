package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	apptui "github.com/lesquel/termino/internal/app/tui"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projecttui "github.com/lesquel/termino/internal/features/project/presentation/tui"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
	"github.com/lesquel/termino/internal/platform/config"
)

func screenFixture() ([]boarddomain.Column, []taskdomain.Task) {
	cols := []boarddomain.Column{
		{ID: "c1", ProjectID: "p1", Name: "Backlog", Position: 0},
		{ID: "c2", ProjectID: "p1", Name: "Doing", Position: 1},
		{ID: "c3", ProjectID: "p1", Name: "Done", Position: 2},
	}
	tasks := []taskdomain.Task{{ID: "t1", ProjectID: "p1", Number: 1, Title: "Hello", ColumnID: "c1"}}
	return cols, tasks
}

// newSizedRoot returns a root model on the board screen, sized and loaded.
func newSizedRoot(t *testing.T, width, height int) (tea.Model, apptui.RootDeps) {
	t.Helper()
	cols, tasks := screenFixture()
	theme, _ := apptui.Load("default")
	deps := apptui.RootDeps{
		Config:  &config.Config{CurrentProjectID: "p1"},
		BoardUC: boardapp.NewBoardUseCases(rootBoardRepo{cols: cols}, rootTaskRepo{tasks: tasks}),
		Theme:   theme,
	}
	var m tea.Model = apptui.NewRootModel(deps)
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = loadBoardInto(t, m, deps)
	return m, deps
}

func loadBoardInto(t *testing.T, m tea.Model, deps apptui.RootDeps) tea.Model {
	t.Helper()
	board, tasksByCol, err := deps.BoardUC.GetBoardWithTasks(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(boardLoaded(board, tasksByCol))
	return m
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func maxLineWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			w = lw
		}
	}
	return w
}

// TestRootEnterOpensDetailWithoutPanic reproduces the slice-bounds panic when
// the detail model was created without the terminal size.
func TestRootEnterOpensDetailWithoutPanic(t *testing.T) {
	m, _ := newSizedRoot(t, 120, 40)
	m, _ = m.Update(key("enter"))
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{ID: "t1", Number: 1, Title: "Hello", ColumnID: "c1"}})

	view := m.View()
	if !strings.Contains(view, "#1 Hello") {
		t.Fatalf("expected detail view with task title, got:\n%s", view)
	}

	m, _ = m.Update(key("j"))
	_ = m.View()
	m, _ = m.Update(key("e"))
	if view := m.View(); !strings.Contains(view, "Edit Task") {
		t.Errorf("expected edit view after e, got:\n%s", view)
	}
}

func TestRootProjectSelectorRendersItemsAndQClosesIt(t *testing.T) {
	m, _ := newSizedRoot(t, 120, 40)
	m, _ = m.Update(key("p"))
	m, _ = m.Update(projecttui.ProjectsLoadedMsg{Projects: []projectdomain.Project{
		{ID: "p1", Name: "Alpha", Key: "ALP"},
		{ID: "p2", Name: "Beta", Key: "BET"},
	}})
	if view := m.View(); !strings.Contains(view, "Alpha") || !strings.Contains(view, "Beta") {
		t.Fatalf("selector should list projects, got:\n%s", view)
	}

	var cmd tea.Cmd
	m, cmd = m.Update(key("q"))
	if isQuit(cmd) {
		t.Fatal("q in the project selector quit the app")
	}
	if view := m.View(); !strings.Contains(view, "Backlog") {
		t.Errorf("q should return to the board, got:\n%s", view)
	}
}

func TestRootBoardAfterProjectSwitchUsesWindowSize(t *testing.T) {
	m, deps := newSizedRoot(t, 120, 40)
	m, _ = m.Update(projecttui.ProjectSelectedMsg{ProjectID: "p1"})
	m = loadBoardInto(t, m, deps)

	view := m.View()
	for _, col := range []string{"Backlog", "Doing", "Done"} {
		if !strings.Contains(view, col) {
			t.Errorf("expected multi-column board with %q after switch, got:\n%s", col, view)
		}
	}
}

func TestRootActivityPanelFitsTerminalWidth(t *testing.T) {
	const width = 130
	m, _ := newSizedRoot(t, width, 40)
	m, _ = m.Update(apptui.ActivityLoadedMsg{Entries: []taskdomain.Activity{
		{TaskID: "t1", ActorID: "a-really-long-agent-identifier", Action: "moved", CreatedAt: time.Now()},
		{TaskID: "t1", ActorID: "cli", Action: "created", CreatedAt: time.Now()},
	}})
	view := m.View()
	if !strings.Contains(view, "Activity") {
		t.Fatalf("expected activity panel at width %d, got:\n%s", width, view)
	}
	if got := maxLineWidth(view); got > width {
		t.Errorf("view is %d columns wide, terminal is %d", got, width)
	}
}

func TestRootActivityEntryShowsActorActionAndTask(t *testing.T) {
	m, _ := newSizedRoot(t, 130, 40)
	m, _ = m.Update(apptui.ActivityLoadedMsg{
		Entries: []taskdomain.Activity{{TaskID: "t1", ActorID: "kiro", Action: "moved", CreatedAt: time.Now()}},
		Labels:  map[string]string{"t1": "#1"},
	})
	if view := m.View(); !strings.Contains(view, "kiro moved #1") {
		t.Errorf("expected 'kiro moved #1' in activity panel, got:\n%s", view)
	}
}
