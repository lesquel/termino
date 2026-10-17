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
