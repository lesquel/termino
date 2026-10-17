package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
	"github.com/lesquel/termino/internal/tuikit"
)

type fakeCreator struct {
	input  taskapp.CreateTaskInput
	labels []string
}

func (f *fakeCreator) CreateTask(_ context.Context, _ taskapp.Actor, in taskapp.CreateTaskInput) (taskdomain.Task, error) {
	f.input = in
	return taskdomain.Task{ID: "t1", Number: 1, Title: in.Title, Priority: in.Priority}, nil
}

func (f *fakeCreator) AddLabels(_ context.Context, _ taskapp.Actor, _ string, names []string) error {
	f.labels = names
	return nil
}

func typeText(m tea.Model, s string) tea.Model {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

func sendKey(m tea.Model, k tea.KeyType) tea.Model {
	m, _ = m.Update(tea.KeyMsg{Type: k})
	return m
}

func TestFormShowsPriorityAndLabelsFields(t *testing.T) {
	var m tea.Model = tasktui.NewFormModel("p1", "c1", &fakeCreator{}, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
	view := ansi.Strip(m.View())
	for _, want := range []string{"Title:", "Priority:", "medium", "Labels:", "tab"} {
		if !strings.Contains(view, want) {
			t.Errorf("form view is missing %q:\n%s", want, view)
		}
	}
}

func TestFormSubmitsPriorityAndLabels(t *testing.T) {
	creator := &fakeCreator{}
	var m tea.Model = tasktui.NewFormModel("p1", "c1", creator, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
	m = typeText(m, "Ship it")
	m = sendKey(m, tea.KeyTab)   // → priority
	m = sendKey(m, tea.KeyRight) // medium → high
	m = sendKey(m, tea.KeyTab)   // → labels
	m = typeText(m, "bug, ui")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should submit the form")
	}
	msg := cmd()
	created, ok := msg.(tasktui.TaskCreatedMsg)
	if !ok {
		t.Fatalf("got %T, want TaskCreatedMsg", msg)
	}
	if creator.input.Title != "Ship it" || creator.input.Priority != taskdomain.PriorityHigh {
		t.Errorf("input = %+v, want title 'Ship it' priority high", creator.input)
	}
	if strings.Join(creator.labels, "|") != "bug|ui" {
		t.Errorf("labels = %q, want [bug ui]", creator.labels)
	}
	if created.Task.Number != 1 {
		t.Errorf("created task = %+v", created.Task)
	}
}

func TestFormTabCyclesBackToTitle(t *testing.T) {
	creator := &fakeCreator{}
	var m tea.Model = tasktui.NewFormModel("p1", "c1", creator, taskapp.Actor{ID: "cli"}, buildTaskTestTheme(), tuikit.DefaultKeyMap())
	for i := 0; i < 3; i++ {
		m = sendKey(m, tea.KeyTab)
	}
	m = typeText(m, "Back on title")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should submit")
	}
	cmd()
	if creator.input.Title != "Back on title" || creator.input.Priority != taskdomain.PriorityMedium {
		t.Errorf("input = %+v", creator.input)
	}
	if len(creator.labels) != 0 {
		t.Errorf("no labels expected, got %q", creator.labels)
	}
}
