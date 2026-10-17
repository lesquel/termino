package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
)

// The task detail is a dialog over the board: the board header and the
// other columns stay visible around it.
func TestRootDetailIsADialogOverTheBoard(t *testing.T) {
	m, _ := newSizedRoot(t, 100, 30)
	m, _ = m.Update(key("enter"))
	m, _ = m.Update(tasktui.TaskLoadedMsg{Task: taskdomain.Task{ID: "t1", Number: 1, Title: "Hello", ColumnID: "c1"}})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.Contains(lines[0], "termino") {
		t.Errorf("the board header should stay visible:\n%s", strings.Join(lines, "\n"))
	}
	view := strings.Join(lines, "\n")
	for _, want := range []string{"╭", "#1 Hello", "esc close", "close esc / enter", "Done"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	m, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("enter should close the detail")
	}
	m, _ = m.Update(cmd())
	if strings.Contains(ansi.Strip(m.View()), "esc close") {
		t.Error("the detail is still open after enter")
	}
}

// Every overlay fits the terminal, from 100x30 down to tiny sizes, with the
// board drawn around it.
func TestRootOverlaysFitEverySize(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 20}, {30, 10}} {
		for _, k := range []string{"enter", "?", "p", "n", "s"} {
			m, _ := newSizedRoot(t, size[0], size[1])
			m, _ = m.Update(key(k))
			m, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			if len(lines) > size[1] {
				t.Errorf("%q %dx%d: view is %d rows", k, size[0], size[1], len(lines))
			}
			if w := maxLineWidth(strings.Join(lines, "\n")); w > size[0] {
				t.Errorf("%q %dx%d: view is %d cells wide", k, size[0], size[1], w)
			}
			if size[1] >= 20 && !strings.Contains(lines[0], "termino") {
				t.Errorf("%q %dx%d: board header hidden:\n%s", k, size[0], size[1], strings.Join(lines, "\n"))
			}
		}
	}
}
