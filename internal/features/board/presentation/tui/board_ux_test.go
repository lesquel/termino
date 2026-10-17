package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/tuikit"
)

// newProjectModel is newLoadedModel for a board that knows its project.
func newProjectModel(t *testing.T, uc *boardapp.BoardUseCases, svc boardtui.Services, width int, now func() time.Time) tea.Model {
	t.Helper()
	m := boardtui.NewBoardModel("proj-1", uc, nil, buildTestTheme(), tuikit.DefaultKeyMap()).
		WithServices(svc).
		WithProject("Web App", "WEB")
	if now != nil {
		m = m.WithClock(now)
	}
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return run(t, model, m.Reload())
}

func plainLines(view string) []string {
	return strings.Split(ansi.Strip(view), "\n")
}

func TestBoardHeaderShowsProjectNameAndKey(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	view := ansi.Strip(newProjectModel(t, uc, boardtui.Services{}, 120, nil).View())
	first := plainLines(view)[0]
	if !strings.Contains(first, "termino · Web App (WEB)") {
		t.Errorf("first line = %q, want project header", first)
	}
}

func TestBoardHeaderShowsActiveFilter(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := newProjectModel(t, uc, boardtui.Services{}, 120, nil)
	m = press(t, m, "/", "d", "o", "c", "s", "enter")
	first := plainLines(m.View())[0]
	if !strings.Contains(first, "Web App (WEB)") || !strings.Contains(first, "Filter: docs") {
		t.Errorf("first line = %q, want header with filter", first)
	}
}

func TestBoardCardsUseProjectKeyRefs(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	view := ansi.Strip(newProjectModel(t, uc, boardtui.Services{}, 120, nil).View())
	if !strings.Contains(view, "WEB-1 Fix login") || !strings.Contains(view, "WEB-2 Write docs") {
		t.Errorf("expected KEY-N card refs in view:\n%s", view)
	}
	if strings.Contains(view, "#1") {
		t.Errorf("view still uses #N refs:\n%s", view)
	}
}

func TestBoardFooterListsKeysInNormalMode(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	lines := plainLines(newProjectModel(t, uc, boardtui.Services{}, 120, nil).View())
	last := lines[len(lines)-1]
	for _, want := range []string{"n new", "enter open", "/ filter", "? help", "q quit"} {
		if !strings.Contains(last, want) {
			t.Errorf("footer %q is missing %q", last, want)
		}
	}
}

func TestBoardFooterFitsWidth(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	for _, width := range []int{80, 60, 40} {
		lines := plainLines(newProjectModel(t, uc, boardtui.Services{}, width, nil).View())
		last := lines[len(lines)-1]
		if w := ansi.StringWidth(last); w > width {
			t.Errorf("width %d: footer is %d cells wide: %q", width, w, last)
		}
		if !strings.Contains(last, "? help") {
			t.Errorf("width %d: footer %q dropped the help hint", width, last)
		}
	}
}

func TestBoardEmptyStateHint(t *testing.T) {
	_, uc := fixture(t, nil, nil)
	view := ansi.Strip(newProjectModel(t, uc, boardtui.Services{}, 120, nil).View())
	if !strings.Contains(view, "No tasks yet — press n to create one") {
		t.Errorf("expected empty-state hint in view:\n%s", view)
	}
}

func TestBoardNoMatchesWhenFiltered(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := newProjectModel(t, uc, boardtui.Services{}, 120, nil)
	m = press(t, m, "/", "z", "z", "z", "enter")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "No matches") {
		t.Errorf("expected 'No matches' in view:\n%s", view)
	}
	if strings.Contains(view, "No tasks yet") {
		t.Errorf("filtered board must not claim there are no tasks:\n%s", view)
	}
}

func TestBoardStatusRowKeepsLayoutAndAutoClears(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	actions := &fakeTaskActions{}
	m := newProjectModel(t, uc, boardtui.Services{Tasks: actions}, 120, clock)
	before := len(plainLines(m.View()))

	m = press(t, m, "L")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "WEB-1 moved to In Progress") {
		t.Fatalf("expected status after move:\n%s", view)
	}
	if after := len(plainLines(m.View())); after != before {
		t.Errorf("status changed the layout height: %d lines → %d", before, after)
	}

	now = now.Add(4 * time.Second)
	m, _ = m.Update(tuikit.RefreshMsg{})
	if strings.Contains(ansi.Strip(m.View()), "moved to") {
		t.Errorf("status did not clear after ~3 s:\n%s", ansi.Strip(m.View()))
	}
}

func TestBoardAssignPickerHidesSystemMembers(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	members := fakeMembers{members: []memberdomain.Member{
		{ID: memberdomain.SystemCLIID, Name: "CLI", Kind: memberdomain.KindHuman},
		{ID: memberdomain.SystemID, Name: "System", Kind: memberdomain.KindHuman},
		{ID: "m-ana", Name: "ana", Kind: memberdomain.KindHuman},
	}}
	m := newProjectModel(t, uc, boardtui.Services{Tasks: &fakeTaskActions{}, Members: members}, 120, nil)
	view := ansi.Strip(press(t, m, "a").View())
	if !strings.Contains(view, "ana") {
		t.Fatalf("expected ana in picker:\n%s", view)
	}
	if strings.Contains(view, "CLI (human)") || strings.Contains(view, "System (human)") {
		t.Errorf("picker lists system members:\n%s", view)
	}
}

func TestBoardTaskCreatedShowsFeedback(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := newProjectModel(t, uc, boardtui.Services{}, 120, nil).(boardtui.BoardModel)
	m, _ = m.Created(taskdomain.Task{ID: "t9", Number: 9, Title: "New"})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Created WEB-9") {
		t.Errorf("expected creation feedback:\n%s", view)
	}
}

// Cards show the theme's priority symbol (medium has none), as the setup
// theme preview promises.
func TestBoardCardsShowPrioritySymbol(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	view := ansi.Strip(newProjectModel(t, uc, boardtui.Services{}, 120, nil).View())
	if !strings.Contains(view, "▲ WEB-1 Fix login") || !strings.Contains(view, "▼ WEB-2 Write docs") {
		t.Errorf("expected priority symbols before the card refs:\n%s", view)
	}
}

// Applying a filter selects a matching card even when the selected column
// has none, so enter opens it right away.
func TestBoardFilterSelectsFirstMatchingColumn(t *testing.T) {
	tasks := append(baseTasks(), taskdomain.Task{ID: "t3", ProjectID: "proj-1", Number: 3, Title: "Deploy", ColumnID: "col-3", Priority: taskdomain.PriorityMedium, Position: 1})
	_, uc := fixture(t, tasks, nil)
	m := newProjectModel(t, uc, boardtui.Services{}, 120, nil)
	m = press(t, m, "l", "l") // Done column, which holds only t3
	m = press(t, m, "/", "d", "o", "c", "s", "enter")
	if got := m.(boardtui.BoardModel).SelectedTaskID(); got != "t2" {
		t.Fatalf("selected = %q, want t2 (the only match)", got)
	}
}
