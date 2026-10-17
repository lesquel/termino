package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
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

func TestDetailModel_StripsTerminalInjectionFromTaskText(t *testing.T) {
	model := tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli", Name: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
	task := taskdomain.Task{
		ID:          "task-1",
		Number:      7,
		Title:       "\x1b]0;pwned\x07Deploy",
		Description: "step\x1b]52;c;ZXZpbA==\x07 one",
	}
	updated, _ := model.Update(tasktui.TaskLoadedMsg{
		Task:     task,
		Comments: []taskdomain.Comment{{AuthorID: "kiro", Body: "done\x1b]0;x\x07"}},
	})
	updated, _ = updated.(tea.Model).Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := updated.(tasktui.DetailModel).View()
	for _, bad := range []string{"\x1b]", "\x07", "pwned", "ZXZpbA=="} {
		if strings.Contains(view, bad) {
			t.Errorf("view contains %q: %q", bad, view)
		}
	}
	if !strings.Contains(view, "Deploy") || !strings.Contains(view, "step one") {
		t.Errorf("sanitized text missing from view: %q", view)
	}
}

func TestDetailModel_TinyOrUnknownHeightDoesNotPanic(t *testing.T) {
	task := taskdomain.Task{ID: "task-1", Number: 3, Title: "Small", Description: "a\nb\nc\nd"}
	for _, h := range []int{0, 1, 2, 3, 4} {
		var m tea.Model = tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
		m, _ = m.Update(tasktui.TaskLoadedMsg{Task: task})
		if h > 0 {
			m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: h})
		}
		for i := 0; i < 5; i++ {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		}
		_ = m.View()
	}
}

func TestDetailModel_ScrollAndEdit(t *testing.T) {
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "line")
	}
	task := taskdomain.Task{ID: "task-1", Number: 3, Title: "Long", Description: strings.Join(lines, "\n")}
	var m tea.Model = tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: task})
	if !strings.Contains(m.View(), "#3 Long") {
		t.Fatalf("title should be visible before scrolling:\n%s", m.View())
	}
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	if strings.Contains(m.View(), "#3 Long") {
		t.Errorf("title should scroll out of view after j:\n%s", m.View())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !strings.Contains(m.View(), "Edit Task") {
		t.Errorf("expected edit view after e:\n%s", m.View())
	}
	m, _ = m.Update(tasktui.TaskUpdatedMsg{})
	if strings.Contains(m.View(), "Edit Task") {
		t.Errorf("a saved edit should leave edit mode:\n%s", m.View())
	}
}

func TestDetailModel_ShowsKeyRefAssigneeNameAndLabels(t *testing.T) {
	var m tea.Model = tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap()).
		WithTaskRef(func(n int) string { return fmt.Sprintf("WEB-%d", n) }).
		WithMemberNames(map[string]string{"m-ana": "ana"})
	assignee := "m-ana"
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{
		ID: "task-1", Number: 5, Title: "Login", AssigneeID: &assignee, Labels: []string{"bug", "ui"},
	}})
	view := m.View()
	for _, want := range []string{"WEB-5 Login", "ana", "bug, ui"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view is missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "m-ana") {
		t.Errorf("detail shows the raw assignee ID:\n%s", view)
	}
}

func TestDetailModel_ShowsSavedAfterEdit(t *testing.T) {
	var m tea.Model = tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{ID: "task-1", Number: 1, Title: "T"}})
	m, _ = m.Update(tasktui.TaskUpdatedMsg{})
	if !strings.Contains(m.View(), "Saved") {
		t.Errorf("expected 'Saved' feedback:\n%s", m.View())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if strings.Contains(m.View(), "Saved") {
		t.Errorf("'Saved' should clear on the next key:\n%s", m.View())
	}
}
