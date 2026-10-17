package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	apptui "github.com/lesquel/termino/internal/app/tui"
	"github.com/lesquel/termino/internal/tuikit"
)

func helpView(t *testing.T, w, h int) []string {
	t.Helper()
	theme, err := tuikit.Load("default", "")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := apptui.NewHelpModel(theme, tuikit.DefaultKeyMap()).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return strings.Split(ansi.Strip(m.View()), "\n")
}

// Each binding stays on one line (key and description together) and the
// overlay fits the terminal, from 100x30 down to 60x20.
func TestHelpFitsAndNeverSplitsADescription(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 20}} {
		lines := helpView(t, size[0], size[1])
		if len(lines) > size[1] {
			t.Errorf("%dx%d: help is %d lines tall", size[0], size[1], len(lines))
		}
		joined := strings.Join(lines, "\n")
		for _, want := range []string{"agent:", "edit (in task detail)", "delete (asks y/N)", "project selector"} {
			found := false
			for _, l := range lines {
				if strings.Contains(l, want) {
					found = true
				}
				if w := ansi.StringWidth(l); w > size[0] {
					t.Fatalf("%dx%d: line is %d cells wide: %q", size[0], size[1], w, l)
				}
			}
			if !found {
				t.Errorf("%dx%d: %q not on a single line:\n%s", size[0], size[1], want, joined)
			}
		}
	}
}
