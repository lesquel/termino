package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	apptui "github.com/lesquel/termino/internal/app/tui"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
)

// TestRootRefreshTickSurvivesOverlays proves the self-rescheduling poll tick
// keeps running while an overlay (help, detail, selector) is open.
func TestRootRefreshTickSurvivesOverlays(t *testing.T) {
	for _, overlay := range []string{"?", "enter", "p"} {
		t.Run(overlay, func(t *testing.T) {
			m, _ := newSizedRoot(t, 120, 40)
			m, _ = m.Update(key(overlay))
			_, cmd := m.Update(apptui.RefreshMsg{})
			if cmd == nil {
				t.Fatalf("refresh tick dropped behind overlay %q: live polling stops", overlay)
			}
		})
	}
}

// TestRootBoardReceivesChangesBehindDetail proves an external change loaded
// while the detail view is open shows once the user goes back.
func TestRootBoardReceivesChangesBehindDetail(t *testing.T) {
	m, _ := newSizedRoot(t, 120, 40)
	m, _ = m.Update(key("enter"))

	cols, tasks := screenFixture()
	tasks = append(tasks, taskdomain.Task{ID: "t2", ProjectID: "p1", Number: 2, Title: "Fresh", ColumnID: "c1", Position: 2})
	uc := boardapp.NewBoardUseCases(rootBoardRepo{cols: cols}, rootTaskRepo{tasks: tasks})
	board, byCol, err := uc.GetBoardWithTasks(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(boardLoaded(board, byCol))
	m, _ = m.Update(tasktui.GoBackMsg{})
	if view := m.View(); !strings.Contains(view, "#2 Fresh") {
		t.Errorf("change loaded behind the detail view is missing:\n%s", view)
	}
}

// TestRootDetailShowsColumnName proves the detail view names the column
// instead of printing its ID.
func TestRootDetailShowsColumnName(t *testing.T) {
	m, _ := newSizedRoot(t, 120, 40)
	m, _ = m.Update(key("enter"))
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{ID: "t1", Number: 1, Title: "Hello", ColumnID: "c1"}})
	lines := strings.Split(m.View(), "\n")
	line := ""
	for i, l := range lines {
		if strings.Contains(l, "esc close") && i+1 < len(lines) {
			line = lines[i+1] // the dialog subtitle
		}
	}
	if !strings.Contains(line, "Backlog") || strings.Contains(line, "c1") {
		t.Errorf("detail should show the column name, got %q", line)
	}
}

// TestRootDetailEditHintMatchesKeys proves the edit hint names a key that
// saves from the description field, where enter inserts a newline.
func TestRootDetailEditHintMatchesKeys(t *testing.T) {
	m, _ := newSizedRoot(t, 120, 40)
	m, _ = m.Update(key("enter"))
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{ID: "t1", Number: 1, Title: "Hello", ColumnID: "c1"}})
	m, _ = m.Update(key("e"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m.View()
	if strings.Contains(view, "enter=save") || !strings.Contains(view, "ctrl+s=save") {
		t.Errorf("description field hint should advertise ctrl+s=save:\n%s", view)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); cmd == nil {
		t.Error("ctrl+s in the description field should save")
	}
}
