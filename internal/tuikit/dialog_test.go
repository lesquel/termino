package tuikit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesquel/termino/internal/tuikit"
	"github.com/muesli/termenv"
)

func dialogTheme(t *testing.T) *tuikit.Theme {
	t.Helper()
	th, err := tuikit.Load("default", "")
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line %02d", i+1)
	}
	return out
}

// Every row of the box has the same width, never wider than the terminal
// minus two cells, at the usual sizes and at tiny ones.
func TestDialogFitsTerminal(t *testing.T) {
	th := dialogTheme(t)
	d := tuikit.Dialog{
		Title:  "PROJ-1 a title long enough to need truncation on small terminals",
		Badge:  "esc close",
		Body:   numbered(40),
		Footer: "scroll ↑↓ · edit e · close esc / enter",
	}
	for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 20}, {30, 10}, {12, 5}} {
		out := strings.Split(ansi.Strip(d.Render(th, size[0], size[1])), "\n")
		if len(out) > size[1] {
			t.Errorf("%dx%d: dialog is %d rows tall", size[0], size[1], len(out))
		}
		w := ansi.StringWidth(out[0])
		if w > size[0]-2 {
			t.Errorf("%dx%d: dialog is %d cells wide", size[0], size[1], w)
		}
		for i, l := range out {
			if ansi.StringWidth(l) != w {
				t.Errorf("%dx%d: row %d is %d cells, want %d: %q", size[0], size[1], i, ansi.StringWidth(l), w, l)
			}
		}
	}
}

// The reference layout: rounded border, bold title left, badge right,
// subtitle, body and a footer hint.
func TestDialogLayout(t *testing.T) {
	d := tuikit.Dialog{Title: "PROJ-1 hola", Subtitle: "Backlog", Badge: "esc close", Body: []string{"body"}, Footer: "close esc"}
	out := strings.Split(ansi.Strip(d.Render(dialogTheme(t), 100, 30)), "\n")
	if !strings.HasPrefix(out[0], "╭") || !strings.HasPrefix(out[len(out)-1], "╰") {
		t.Fatalf("expected a rounded border:\n%s", strings.Join(out, "\n"))
	}
	if !strings.HasPrefix(out[1], "│ PROJ-1 hola") || !strings.HasSuffix(out[1], "[esc close] │") {
		t.Errorf("header should hold the title left and the badge right: %q", out[1])
	}
	want := []string{"Backlog", "", "body", "", "close esc"}
	for i, w := range want {
		if got := strings.TrimSpace(strings.Trim(out[2+i], "│")); got != w {
			t.Errorf("row %d = %q, want %q", 2+i, got, w)
		}
	}
}

// With colors on, every cell inside the border carries the panel
// background, even right after a nested styled span resets the style.
func TestDialogFillsEveryCell(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	th := dialogTheme(t)
	span := lipgloss.NewStyle().Foreground(th.Accent).Render("key")
	d := tuikit.Dialog{Title: "T", Badge: "esc close", Body: []string{span + " plain text", "x"}, Footer: "hint"}
	out := d.Render(th, 80, 24)
	bg := strings.TrimSuffix(lipgloss.NewStyle().Background(th.SurfaceAlt).Render("x"), "x\x1b[0m")
	if bg == "" {
		t.Fatal("could not derive the background sequence")
	}
	for i, l := range strings.Split(out, "\n") {
		for _, reset := range []string{"\x1b[0m ", "\x1b[m "} {
			if strings.Contains(l, reset) {
				t.Errorf("row %d has a cell without the panel background: %q", i, l)
			}
		}
	}
	if !strings.Contains(out, "\x1b[0m"+bg) {
		t.Error("the background is not re-applied after a nested span")
	}
}

// A body taller than the box scrolls: the offset picks the first row, an
// indicator shows the position, and out-of-range offsets are clamped.
func TestDialogScrolls(t *testing.T) {
	th := dialogTheme(t)
	d := tuikit.Dialog{Title: "T", Body: numbered(50), Footer: "hint", Scroll: 5}
	out := ansi.Strip(d.Render(th, 80, 20))
	if strings.Contains(out, "line 05") || !strings.Contains(out, "line 06") {
		t.Errorf("scroll 5 should start at line 06:\n%s", out)
	}
	if !strings.Contains(out, "↑↓") {
		t.Errorf("a scrollable body needs an indicator:\n%s", out)
	}
	maxScroll := d.MaxScroll(80, 20)
	if maxScroll <= 0 || maxScroll >= 50 {
		t.Fatalf("MaxScroll = %d", maxScroll)
	}
	d.Scroll = 1000
	if out := ansi.Strip(d.Render(th, 80, 20)); !strings.Contains(out, "line 50") {
		t.Errorf("an overflowing offset should clamp to the end:\n%s", out)
	}
	short := tuikit.Dialog{Title: "T", Body: numbered(3)}
	if short.MaxScroll(80, 20) != 0 || strings.Contains(ansi.Strip(short.Render(th, 80, 20)), "↑↓") {
		t.Error("a body that fits must not scroll")
	}
}

func TestDialogZeroThemeAndSizeDoNotPanic(t *testing.T) {
	d := tuikit.Dialog{Title: "T", Badge: "b", Body: numbered(3), Footer: "f"}
	for _, size := range [][2]int{{0, 0}, {1, 1}, {3, 2}, {6, 3}} {
		_ = d.Render(&tuikit.Theme{}, size[0], size[1])
		_ = d.MaxScroll(size[0], size[1])
		_ = d.BodyRoom(size[0], size[1])
	}
}

// Overlay centers the dialog over the background and keeps the background
// visible on every side.
func TestOverlayCentersOverBackground(t *testing.T) {
	bg := strings.TrimSuffix(strings.Repeat(strings.Repeat("x", 20)+"\n", 10), "\n")
	out := strings.Split(tuikit.Overlay(bg, "ab\ncd", 20, 10), "\n")
	if len(out) != 10 {
		t.Fatalf("overlay has %d rows, want 10", len(out))
	}
	if out[4] != "xxxxxxxxxabxxxxxxxxx" && ansi.Strip(out[4]) != "xxxxxxxxxabxxxxxxxxx" {
		t.Errorf("row 4 = %q", ansi.Strip(out[4]))
	}
	if ansi.Strip(out[5]) != "xxxxxxxxxcdxxxxxxxxx" {
		t.Errorf("row 5 = %q", ansi.Strip(out[5]))
	}
	if ansi.Strip(out[0]) != strings.Repeat("x", 20) {
		t.Errorf("rows outside the dialog must keep the background: %q", out[0])
	}
}

// A short or empty background is padded to the screen, and a dialog larger
// than the screen is clipped instead of panicking.
func TestOverlayPadsAndClips(t *testing.T) {
	out := strings.Split(ansi.Strip(tuikit.Overlay("hi", "[]", 10, 4)), "\n")
	if len(out) != 4 || !strings.Contains(out[1], "[]") || !strings.HasPrefix(out[0], "hi") {
		t.Errorf("unexpected overlay: %q", out)
	}
	big := strings.Repeat(strings.Repeat("#", 30)+"\n", 12)
	for _, l := range strings.Split(ansi.Strip(tuikit.Overlay("", big, 10, 4)), "\n") {
		if ansi.StringWidth(l) > 10 {
			t.Errorf("clipped row is %d cells: %q", ansi.StringWidth(l), l)
		}
	}
	if got := tuikit.Overlay("", "ab\ncd", 0, 0); ansi.Strip(got) != "ab\ncd" {
		t.Errorf("unknown size should return the dialog as is, got %q", got)
	}
}
