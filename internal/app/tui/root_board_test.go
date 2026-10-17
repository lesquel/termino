package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	apptui "github.com/lesquel/termino/internal/app/tui"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/config"
)

type rootBoardRepo struct{ cols []boarddomain.Column }

func (r rootBoardRepo) SaveColumn(context.Context, boarddomain.Column) error { return nil }
func (r rootBoardRepo) FindColumnByID(context.Context, string) (boarddomain.Column, error) {
	return boarddomain.Column{}, boarddomain.ErrColumnNotFound
}
func (r rootBoardRepo) ListColumns(context.Context, string) ([]boarddomain.Column, error) {
	return r.cols, nil
}
func (r rootBoardRepo) ReorderColumns(context.Context, string, []string) error { return nil }
func (r rootBoardRepo) DeleteColumn(context.Context, string) error             { return nil }

type rootTaskRepo struct{ tasks []taskdomain.Task }

func (r rootTaskRepo) Save(context.Context, taskdomain.Task) error { return nil }
func (r rootTaskRepo) FindByID(context.Context, string) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (r rootTaskRepo) FindByNumber(context.Context, string, int) (taskdomain.Task, error) {
	return taskdomain.Task{}, nil
}
func (r rootTaskRepo) List(context.Context, taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	return r.tasks, nil
}
func (r rootTaskRepo) SoftDelete(context.Context, string, time.Time) error { return nil }
func (r rootTaskRepo) Restore(context.Context, string) error               { return nil }
func (r rootTaskRepo) NextNumber(context.Context, string) (int, error)     { return 1, nil }

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// TestRootBoardFilterCapturesGlobalKeys proves that while the board filter
// prompt is open, "q" and "n" are typed into the prompt instead of quitting
// or opening the new-task form.
func TestRootBoardFilterCapturesGlobalKeys(t *testing.T) {
	cols := []boarddomain.Column{{ID: "c1", ProjectID: "p1", Name: "Backlog"}}
	tasks := []taskdomain.Task{{ID: "t1", ProjectID: "p1", Number: 1, Title: "Hello", ColumnID: "c1"}}
	theme, _ := apptui.Load("default")
	deps := apptui.RootDeps{
		Config:  &config.Config{CurrentProjectID: "p1", Motion: true},
		BoardUC: boardapp.NewBoardUseCases(rootBoardRepo{cols: cols}, rootTaskRepo{tasks: tasks}),
		Theme:   theme,
	}
	var m tea.Model = apptui.NewRootModel(deps)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	board, tasksByCol, err := deps.BoardUC.GetBoardWithTasks(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(boardLoaded(board, tasksByCol))

	for _, k := range []string{"/", "q", "n", "enter"} {
		var cmd tea.Cmd
		m, cmd = m.Update(key(k))
		if k == "q" && cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatal("q quit the program while the filter prompt was open")
			}
		}
	}
	if view := m.View(); !strings.Contains(view, "Filter: qn") {
		t.Errorf("expected keys typed into the filter, got view:\n%s", view)
	}
}

func boardLoaded(b boarddomain.Board, tasks map[string][]taskdomain.Task) boardtui.BoardLoadedMsg {
	return boardtui.BoardLoadedMsg{Board: b, Tasks: tasks}
}
