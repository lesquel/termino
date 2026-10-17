package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	apptui "github.com/lesquel/termino/internal/app/tui"
	"github.com/lesquel/termino/internal/tuikit"
	"github.com/lesquel/termino/internal/tuikit/tuikittest"
)

func helpModel(t *testing.T, w, h int) tea.Model {
	t.Helper()
	theme, err := tuikit.Load("default", "")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := apptui.NewHelpModel(theme, tuikit.DefaultKeyMap()).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// helpText scrolls through the whole help dialog and returns every row seen.
func helpText(t *testing.T, w, h int) string {
	t.Helper()
	m := helpModel(t, w, h)
	var seen []string
	for i := 0; i < 80; i++ {
		view := ansi.Strip(m.View())
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			t.Fatalf("%dx%d: help is %d rows tall", w, h, len(lines))
		}
		for _, l := range lines {
			if lw := ansi.StringWidth(l); lw > w-2 {
				t.Fatalf("%dx%d: row is %d cells: %q", w, h, lw, l)
			}
		}
		seen = append(seen, lines...)
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if cmd != nil {
			t.Fatal("j should scroll the help, not close it")
		}
	}
	return strings.Join(seen, "\n")
}

// The help dialog fits every size and, scrolled through, shows every
// section and every entry with its key and description on one row.
func TestHelpListsEveryEntryAndFits(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 20}, {30, 10}} {
		text := helpText(t, size[0], size[1])
		if size[0] < 60 {
			continue // tiny terminals truncate rows; fitting is what matters
		}
		for _, s := range tuikit.HelpSections() {
			if !strings.Contains(text, s.Title) {
				t.Errorf("%dx%d: section %q missing", size[0], size[1], s.Title)
			}
			for _, e := range s.Entries {
				found := false
				for _, l := range strings.Split(text, "\n") {
					if strings.Contains(l, e.Label) && strings.Contains(l, e.Desc) {
						found = true
					}
				}
				if !found {
					t.Errorf("%dx%d: %q %q is not on one row", size[0], size[1], e.Label, e.Desc)
				}
			}
		}
	}
}

// Keys and descriptions line up in two columns.
func TestHelpColumnsAreAligned(t *testing.T) {
	lines := strings.Split(ansi.Strip(helpModel(t, 100, 80).View()), "\n")
	col := -1
	for _, want := range []string{"previous card", "open task detail", "quit from anywhere"} {
		for _, l := range lines {
			if i := strings.Index(l, want); i >= 0 {
				c := ansi.StringWidth(l[:i])
				if col >= 0 && c != col {
					t.Errorf("%q starts at column %d, want %d", want, c, col)
				}
				col = c
			}
		}
	}
	if col < 0 {
		t.Fatal("entries not found")
	}
}

// Any key other than scrolling closes the help.
func TestHelpClosesOnOtherKeys(t *testing.T) {
	_, cmd := helpModel(t, 100, 30).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should close the help")
	}
	if _, ok := cmd().(tuikit.DismissHelpMsg); !ok {
		t.Error("esc should send DismissHelpMsg")
	}
}

// Every key the root and the settings screen switch on is in the help.
func TestRootAndSettingsKeysAreDocumented(t *testing.T) {
	tuikittest.AssertDocumented(t, tuikittest.CaseKeys(t, "root.go", "Update"),
		tuikit.HelpNavigate, tuikit.HelpTasks, tuikit.HelpViews, tuikit.HelpGeneral)
	tuikittest.AssertDocumented(t, tuikittest.CaseKeys(t, "settings.go", "Update"), tuikit.HelpSettings)
	tuikittest.AssertDocumented(t, tuikittest.CaseKeys(t, "help.go", "Update"), tuikit.HelpGeneral)
}

// Several j presses delivered as one message scroll instead of closing.
func TestHelpScrollBurstDoesNotClose(t *testing.T) {
	m, cmd := helpModel(t, 30, 10).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	if cmd != nil {
		t.Fatal("a j burst closed the help")
	}
	if !strings.Contains(ansi.Strip(m.View()), "3-") {
		t.Errorf("a j burst should scroll two rows:\n%s", ansi.Strip(m.View()))
	}
}
