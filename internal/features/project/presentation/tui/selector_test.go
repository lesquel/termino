package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projecttui "github.com/lesquel/termino/internal/features/project/presentation/tui"
	"github.com/lesquel/termino/internal/tuikit"
)

func loadedSelector(t *testing.T, current string, projects ...projectdomain.Project) projecttui.SelectorModel {
	t.Helper()
	theme, err := tuikit.Load("default", "")
	if err != nil {
		t.Fatal(err)
	}
	var m tea.Model = projecttui.NewSelectorModel(nil, theme).WithCurrent(current)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(projecttui.ProjectsLoadedMsg{Projects: projects})
	return m.(projecttui.SelectorModel)
}

func TestSelectorMarksCurrentProject(t *testing.T) {
	m := loadedSelector(t, "p2",
		projectdomain.Project{ID: "p1", Name: "Alpha", Key: "ALP"},
		projectdomain.Project{ID: "p2", Name: "Beta", Key: "BET"},
	)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Beta  (current)") {
		t.Errorf("current project not marked:\n%s", view)
	}
	if strings.Contains(view, "Alpha  (current)") {
		t.Errorf("non-current project marked:\n%s", view)
	}
}

func TestSelectorQHintSaysBackWhenABoardIsOpen(t *testing.T) {
	m := loadedSelector(t, "p1", projectdomain.Project{ID: "p1", Name: "Alpha", Key: "ALP"})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "q back") || strings.Contains(view, "q quit") {
		t.Errorf("expected 'q back' hint:\n%s", view)
	}
}

func TestSelectorEmptyPointsToSetup(t *testing.T) {
	m := loadedSelector(t, "")
	if view := m.View(); !strings.Contains(view, "termino setup") {
		t.Errorf("empty selector should point to termino setup:\n%s", view)
	}
}

// The selector is a dialog that fits every supported size.
func TestSelectorIsADialogThatFits(t *testing.T) {
	theme, err := tuikit.Load("default", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 20}, {30, 10}} {
		var m tea.Model = projecttui.NewSelectorModel(nil, theme).WithCurrent("p1")
		m, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m, _ = m.Update(projecttui.ProjectsLoadedMsg{Projects: []projectdomain.Project{
			{ID: "p1", Name: "Alpha", Key: "ALP"}, {ID: "p2", Name: "Beta", Key: "BET"},
		}})
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if !strings.HasPrefix(lines[0], "╭") || !strings.Contains(lines[1], "Select a") {
			t.Fatalf("%dx%d: expected a titled dialog:\n%s", size[0], size[1], strings.Join(lines, "\n"))
		}
		if len(lines) > size[1] {
			t.Errorf("%dx%d: %d rows tall", size[0], size[1], len(lines))
		}
		for _, l := range lines {
			if ansi.StringWidth(l) > size[0]-2 {
				t.Errorf("%dx%d: row is %d cells: %q", size[0], size[1], ansi.StringWidth(l), l)
			}
		}
	}
}
