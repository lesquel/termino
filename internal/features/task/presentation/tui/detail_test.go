package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/tuikit"
)

func buildTaskTestTheme() *tuikit.Theme {
	th, err := tuikit.Load("default", "")
	if err != nil {
		th = &tuikit.Theme{
			Accent:      lipgloss.Color("#89B4FA"),
			Text:        lipgloss.Color("#CDD6F4"),
			TextMuted:   lipgloss.Color("#585B70"),
			Danger:      lipgloss.Color("#F38BA8"),
			Surface:     lipgloss.Color("#1E1E2E"),
			BorderStyle: lipgloss.RoundedBorder(),
			Border:      lipgloss.Color("#313244"),
		}
	}
	return th
}

func TestDetailModel_LoadedTask_ShowsTitle(t *testing.T) {
	th := buildTaskTestTheme()
	keys := tuikit.DefaultKeyMap()

	model := tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli", Name: "cli"}, th, keys)

	// Inject the loaded message directly.
	task := taskdomain.Task{
		ID:       "task-1",
		Number:   42,
		Title:    "Important work",
		Priority: taskdomain.PriorityHigh,
		ColumnID: "col-1",
	}
	updated, _ := model.Update(tasktui.TaskLoadedMsg{Task: task})
	updated, _ = updated.(tea.Model).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	dm := updated.(tasktui.DetailModel)

	view := dm.View()
	if !strings.Contains(view, "Important work") {
		t.Errorf("expected task title in view, got: %q", view[:min(len(view), 300)])
	}
	if !strings.Contains(view, "#42") {
		t.Errorf("expected task ref #42 in view, got: %q", view[:min(len(view), 300)])
	}
}

func TestDetailModel_ESC_GoesBack(t *testing.T) {
	th := buildTaskTestTheme()
	keys := tuikit.DefaultKeyMap()

	model := tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli", Name: "cli"}, th, keys)
	task := taskdomain.Task{ID: "task-1", Number: 1, Title: "Task"}
	updated, _ := model.Update(tasktui.TaskLoadedMsg{Task: task})

	_, cmd := updated.(tea.Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Error("expected cmd from ESC key")
		return
	}
	msg := cmd()
	if _, ok := msg.(tasktui.GoBackMsg); !ok {
		t.Errorf("expected GoBackMsg, got %T", msg)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
