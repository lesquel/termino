package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
)

// TestRootViewFillsTerminalHeight proves every frame, with or without a
// dialog, is exactly as tall as the terminal. Bubble Tea's line-diff
// renderer skips lines that did not change and then erases the screen below
// the cursor when a frame is shorter than the previous one; when the last
// line (the board footer) was skipped, that erase wipes it. A dialog frame
// is padded to the terminal height, so going back to a shorter board frame
// made the key hints vanish after closing the task detail.
func TestRootViewFillsTerminalHeight(t *testing.T) {
	const width, height = 105, 30
	m, _ := newSizedRoot(t, width, height)

	assertHeight := func(step string, m tea.Model) {
		t.Helper()
		if got := len(strings.Split(m.View(), "\n")); got != height {
			t.Errorf("%s: frame has %d lines, want %d (terminal height)", step, got, height)
		}
	}

	assertHeight("board", m)
	m, _ = m.Update(key("enter"))
	assertHeight("detail open", m)
	m, _ = m.Update(tasktui.GoBackMsg{})
	assertHeight("detail closed", m)
	if view := m.View(); !strings.Contains(view, "q quit") {
		t.Errorf("footer hints missing after closing the detail:\n%s", view)
	}
	m, _ = m.Update(key("?"))
	assertHeight("help open", m)
}
