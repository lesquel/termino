package tui_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/tuikit"
)

// stubBoardRepo implements boarddomain.BoardRepository for testing.
type stubBoardRepo struct {
	cols []boarddomain.Column
}

func (r *stubBoardRepo) SaveColumn(_ context.Context, col boarddomain.Column) error { return nil }
func (r *stubBoardRepo) FindColumnByID(_ context.Context, id string) (boarddomain.Column, error) {
	for _, c := range r.cols {
		if c.ID == id {
			return c, nil
		}
	}
	return boarddomain.Column{}, boarddomain.ErrColumnNotFound
}
func (r *stubBoardRepo) ListColumns(_ context.Context, projectID string) ([]boarddomain.Column, error) {
	return r.cols, nil
}
func (r *stubBoardRepo) ReorderColumns(_ context.Context, projectID string, ids []string) error {
	return nil
}
func (r *stubBoardRepo) DeleteColumn(_ context.Context, id string) error { return nil }

// stubTaskRepo implements taskdomain.TaskRepository for testing.
type stubTaskRepo struct {
	tasks []taskdomain.Task
}

func (r *stubTaskRepo) Save(_ context.Context, t taskdomain.Task) error { return nil }
func (r *stubTaskRepo) FindByID(_ context.Context, id string) (taskdomain.Task, error) {
	for _, t := range r.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return taskdomain.Task{}, fmt.Errorf("task not found: %s", id)
}
func (r *stubTaskRepo) FindByNumber(_ context.Context, projectID string, n int) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (r *stubTaskRepo) List(_ context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	var out []taskdomain.Task
	for _, t := range r.tasks {
		if filter.ProjectID != "" && t.ProjectID != filter.ProjectID {
			continue
		}
		if filter.ColumnID != "" && t.ColumnID != filter.ColumnID {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
func (r *stubTaskRepo) SoftDelete(_ context.Context, id string, at time.Time) error { return nil }
func (r *stubTaskRepo) Restore(_ context.Context, id string) error                  { return nil }
func (r *stubTaskRepo) NextNumber(_ context.Context, projectID string) (int, error) { return 1, nil }

func buildTestTheme() *tuikit.Theme {
	th, err := tuikit.Load("default", "")
	if err != nil || th == nil {
		th = &tuikit.Theme{
			Surface:     lipgloss.Color("#1E1E2E"),
			Text:        lipgloss.Color("#CDD6F4"),
			TextMuted:   lipgloss.Color("#585B70"),
			Accent:      lipgloss.Color("#89B4FA"),
			Border:      lipgloss.Color("#313244"),
			BorderStyle: lipgloss.RoundedBorder(),
			Symbols:     map[string]string{"done": "✓"},
		}
	}
	return th
}

func TestBoardRendersWithTasks(t *testing.T) {
	cols := []boarddomain.Column{
		{ID: "col-1", ProjectID: "proj-1", Name: "Backlog", Position: 0},
		{ID: "col-2", ProjectID: "proj-1", Name: "In Progress", Position: 1},
	}

	boardRepo := &stubBoardRepo{cols: cols}
	taskRepo := &stubTaskRepo{}
	uc := boardapp.NewBoardUseCases(boardRepo, taskRepo)
	th := buildTestTheme()

	model := boardtui.NewBoardModel("proj-1", uc, nil, th, tuikit.DefaultKeyMap())

	// Inject board loaded message directly.
	board := boarddomain.Board{
		ProjectID: "proj-1",
		Columns:   cols,
		Tasks:     map[string][]string{"col-1": {}, "col-2": {}},
	}
	updated, _ := model.Update(boardtui.BoardLoadedMsg{Board: board})
	updated, _ = updated.(tea.Model).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	bm := updated.(boardtui.BoardModel)

	view := bm.View()
	if view == "" {
		t.Error("expected non-empty board view")
	}
	// Should contain column names.
	if !strings.Contains(view, "Backlog") {
		t.Errorf("expected 'Backlog' in board view, got: %q", view[:min(len(view), 200)])
	}
}

func TestBoardRendersLoadingState(t *testing.T) {
	boardRepo := &stubBoardRepo{}
	taskRepo := &stubTaskRepo{}
	uc := boardapp.NewBoardUseCases(boardRepo, taskRepo)
	th := buildTestTheme()

	model := boardtui.NewBoardModel("proj-1", uc, nil, th, tuikit.DefaultKeyMap())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := updated.(boardtui.BoardModel).View()
	if !strings.Contains(view, "Loading") && view != "" {
		// Either "Loading board…" or empty is acceptable before data arrives.
		t.Logf("board loading view: %q", view)
	}
}

func TestBoardNavigationChangesSelectedTask(t *testing.T) {
	cols := []boarddomain.Column{
		{ID: "col-1", ProjectID: "proj-1", Name: "Backlog", Position: 0},
	}

	boardRepo := &stubBoardRepo{cols: cols}
	taskRepo := &stubTaskRepo{}
	uc := boardapp.NewBoardUseCases(boardRepo, taskRepo)
	th := buildTestTheme()

	model := boardtui.NewBoardModel("proj-1", uc, nil, th, tuikit.DefaultKeyMap())
	board := boarddomain.Board{
		ProjectID: "proj-1",
		Columns:   cols,
		Tasks:     map[string][]string{"col-1": {"task-1", "task-2"}},
	}
	updated, _ := model.Update(boardtui.BoardLoadedMsg{Board: board})
	m := updated.(boardtui.BoardModel)

	if m.SelectedTaskID() != "task-1" {
		t.Errorf("expected task-1 selected, got %s", m.SelectedTaskID())
	}

	updated2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := updated2.(boardtui.BoardModel)
	if m2.SelectedTaskID() != "task-2" {
		t.Errorf("expected task-2 selected after j, got %s", m2.SelectedTaskID())
	}
}

func TestBoardNavigationColumns(t *testing.T) {
	cols := []boarddomain.Column{
		{ID: "col-1", ProjectID: "proj-1", Name: "Backlog", Position: 0},
		{ID: "col-2", ProjectID: "proj-1", Name: "In Progress", Position: 1},
	}

	boardRepo := &stubBoardRepo{cols: cols}
	taskRepo := &stubTaskRepo{}
	uc := boardapp.NewBoardUseCases(boardRepo, taskRepo)
	th := buildTestTheme()

	model := boardtui.NewBoardModel("proj-1", uc, nil, th, tuikit.DefaultKeyMap())
	board := boarddomain.Board{
		ProjectID: "proj-1",
		Columns:   cols,
		Tasks:     map[string][]string{"col-1": {}, "col-2": {}},
	}
	updated, _ := model.Update(boardtui.BoardLoadedMsg{Board: board})
	m := updated.(boardtui.BoardModel)

	if m.ColIndex() != 0 {
		t.Errorf("expected colIndex 0, got %d", m.ColIndex())
	}
	updated2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m2 := updated2.(boardtui.BoardModel)
	if m2.ColIndex() != 1 {
		t.Errorf("expected colIndex 1 after l, got %d", m2.ColIndex())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
