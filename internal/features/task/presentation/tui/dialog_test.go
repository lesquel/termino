package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
	"github.com/lesquel/termino/internal/tuikit"
	"github.com/lesquel/termino/internal/tuikit/tuikittest"
)

// Every key the detail view and the form switch on is listed in help.
func TestDetailAndFormKeysAreDocumented(t *testing.T) {
	tuikittest.AssertDocumented(t,
		tuikittest.CaseKeys(t, "detail.go", "handleViewKey", "handleEditKey"),
		tuikit.HelpDetail)
	tuikittest.AssertDocumented(t, tuikittest.CaseKeys(t, "form.go", "Update"), tuikit.HelpForm)
}

func loadedDetail(t *testing.T, w, h int) tea.Model {
	t.Helper()
	var m tea.Model = tasktui.NewDetailModel("task-1", nil, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap()).
		WithColumnNames(map[string]string{"col-1": "Backlog"}).
		WithTaskRef(func(n int) string { return "PROJ-1" })
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{
		ID: "task-1", Number: 1, Title: "hola", ColumnID: "col-1", Priority: taskdomain.PriorityMedium,
		Description: strings.Repeat("a long description line that has to wrap inside the dialog ", 4),
	}})
	return m
}

// The detail is a dialog: title and close badge in the header, the column
// as subtitle and the key hints in the footer.
func TestDetailRendersAsDialog(t *testing.T) {
	lines := strings.Split(ansi.Strip(loadedDetail(t, 100, 30).View()), "\n")
	if !strings.HasPrefix(lines[0], "╭") {
		t.Fatalf("expected a bordered dialog:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[1], "PROJ-1 hola") || !strings.Contains(lines[1], "esc close") {
		t.Errorf("header = %q", lines[1])
	}
	if !strings.Contains(lines[2], "Backlog") {
		t.Errorf("subtitle should name the column: %q", lines[2])
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "close esc / enter") {
		t.Errorf("footer hint missing:\n%s", joined)
	}
	if strings.Contains(joined, "…") {
		t.Errorf("the description should wrap, not be cut:\n%s", joined)
	}
}

// enter closes the detail like esc.
func TestDetailEnterCloses(t *testing.T) {
	_, cmd := loadedDetail(t, 100, 30).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should close the detail")
	}
	if _, ok := cmd().(tasktui.GoBackMsg); !ok {
		t.Error("enter should send GoBackMsg")
	}
}

// The detail and the form fit every supported size, down to tiny ones.
func TestDetailAndFormFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 20}, {30, 10}} {
		form, _ := tasktui.NewFormModel("p1", "c1", &fakeCreator{}, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap()).
			Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		edit, _ := loadedDetail(t, size[0], size[1]).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
		for name, m := range map[string]tea.Model{"detail": loadedDetail(t, size[0], size[1]), "form": form, "edit": edit} {
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			if len(lines) > size[1] {
				t.Errorf("%s %dx%d: %d rows tall", name, size[0], size[1], len(lines))
			}
			for _, l := range lines {
				if ansi.StringWidth(l) > size[0]-2 {
					t.Errorf("%s %dx%d: row is %d cells: %q", name, size[0], size[1], ansi.StringWidth(l), l)
				}
			}
		}
	}
}
