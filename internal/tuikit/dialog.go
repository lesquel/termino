package tuikit

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Dialog is the shared modal panel: a rounded border, a solid background
// on every inner cell, a bold title with a badge on the right, an optional
// subtitle, a scrollable body and a dim footer hint.
//
//	╭──────────────────────────────╮
//	│ PROJ-1 hola     [esc close]  │
//	│ Backlog                      │
//	│                              │
//	│ body ...                     │
//	│                              │
//	│ scroll ↑↓ · close esc        │
//	╰──────────────────────────────╯
type Dialog struct {
	Title    string
	Subtitle string
	Badge    string   // short key hint drawn top-right, e.g. "esc close"
	Body     []string // pre-rendered lines; may carry their own styles
	Footer   string
	Width    int // preferred inner width; 0 fits the content
	MaxWidth int // outer width cap; 0 uses about 70% of the terminal
	Scroll   int // first visible body row; clamped when rendering
	// Border overrides the border color (e.g. Danger for a delete prompt);
	// empty uses the theme accent.
	Border lipgloss.Color
}

const (
	dialogFrameW   = 4  // border plus one cell of padding on each side
	dialogMinOuter = 24 // narrowest box drawn when the terminal allows it
	unknownTermW   = 80 // width assumed before the first resize
	// From this terminal height on, a dialog leaves the top and bottom
	// rows to the background.
	dialogMarginH = 12
)

// dialogLayout is the computed geometry of a dialog on a terminal.
type dialogLayout struct {
	outer, inner int
	bodyH        int
	subtitle     bool
	footer       bool
	gaps         bool // blank rows around the body
}

// Inner returns the width available to body lines on a termW-wide terminal.
func (d Dialog) Inner(termW int) int { return d.layout(termW, 0, 0).inner }

// BodyRoom returns how many body rows fit on a termW×termH terminal, as if
// the body were unbounded (used to size lists inside the dialog).
func (d Dialog) BodyRoom(termW, termH int) int {
	return d.layout(termW, termH, -1).bodyH
}

// MaxScroll returns the largest useful Scroll value for the current body.
func (d Dialog) MaxScroll(termW, termH int) int {
	l := d.layout(termW, termH, len(d.Body))
	return max(len(d.Body)-l.bodyH, 0)
}

// layout sizes the box. bodyLen < 0 means "as many rows as fit"; termH <= 0
// means the height is unknown and the whole body is shown.
func (d Dialog) layout(termW, termH, bodyLen int) dialogLayout {
	if termW <= 0 {
		termW = unknownTermW
	}
	content := d.Width
	if content <= 0 {
		content = ansi.StringWidth(d.Title)
		if d.Badge != "" {
			content += ansi.StringWidth(badgeText(d.Badge, true)) + 2
		}
		content = max(content, ansi.StringWidth(d.Subtitle), ansi.StringWidth(d.Footer))
		for _, l := range d.Body {
			content = max(content, ansi.StringWidth(l))
		}
	}
	limit := max(termW-2, 1)
	ceiling := d.MaxWidth
	if ceiling <= 0 {
		ceiling = max(termW*7/10, min(60, limit))
	}
	ceiling = min(ceiling, limit)
	outer := min(content+dialogFrameW, ceiling)
	outer = max(outer, min(dialogMinOuter, limit))
	l := dialogLayout{outer: outer, inner: max(outer-dialogFrameW, 1)}

	l.subtitle = d.Subtitle != ""
	l.footer = d.Footer != ""
	l.gaps = bodyLen != 0
	if termH <= 0 {
		l.bodyH = max(bodyLen, 0)
		return l
	}
	if termH >= dialogMarginH {
		termH -= 2 // keep a background row free above and below
	}
	room := func() int {
		n := termH - 3 // border rows and the header
		if l.subtitle {
			n--
		}
		if l.footer {
			n--
		}
		if l.gaps {
			n -= 2
		}
		return n
	}
	// Drop decoration, least useful first, until one body row fits.
	if room() < 1 {
		l.gaps = false
	}
	if room() < 1 {
		l.subtitle = false
	}
	if room() < 1 {
		l.footer = false
	}
	l.bodyH = max(room(), 0)
	if bodyLen >= 0 {
		l.bodyH = min(l.bodyH, bodyLen)
	}
	return l
}

// badgeText is the badge label; plain output brackets it so it still reads
// as a badge without a background.
func badgeText(badge string, plain bool) string {
	if plain {
		return "[" + badge + "]"
	}
	return " " + badge + " "
}

// colorless reports whether lipgloss drops colors (NO_COLOR or a terminal
// without color support); dialogs are then border and text only.
func colorless() bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	probe := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Render("x")
	return probe == "x"
}

// Render draws the dialog for a termW×termH terminal (0 = unknown). The
// result is the box only; place it with Overlay.
func (d Dialog) Render(t *Theme, termW, termH int) string {
	if t == nil {
		t = &Theme{}
	}
	plain := colorless()
	l := d.layout(termW, termH, len(d.Body))
	panel := lipgloss.NewStyle()
	if !plain {
		panel = panel.Background(t.SurfaceAlt)
	}
	// bgSeq re-opens the panel background after a nested span resets it.
	bgSeq := ""
	if !plain && t.SurfaceAlt != "" {
		bgSeq, _, _ = strings.Cut(panel.Render("x"), "x")
	}
	borderColor := t.Accent
	if d.Border != "" {
		borderColor = d.Border
	}
	border := panel.Foreground(borderColor)
	b := t.BorderStyle
	if b.Top == "" {
		b = lipgloss.RoundedBorder()
	}

	row := func(content string) string {
		content = ansi.Truncate(content, l.inner, "…")
		pad := l.inner - ansi.StringWidth(content)
		content = strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+bgSeq)
		content = strings.ReplaceAll(content, "\x1b[m", "\x1b[0m"+bgSeq)
		return border.Render(b.Left) + bgSeq + " " + content + strings.Repeat(" ", max(pad, 0)) + " " +
			border.Render(b.Right)
	}

	rows := []string{border.Render(b.TopLeft + strings.Repeat(b.Top, max(l.outer-2, 0)) + b.TopRight)}
	rows = append(rows, row(d.header(t, panel, l.inner, plain)))
	if l.subtitle {
		rows = append(rows, row(panel.Foreground(t.TextMuted).Render(d.Subtitle)))
	}
	if l.gaps {
		rows = append(rows, row(""))
	}
	start := min(max(d.Scroll, 0), max(len(d.Body)-l.bodyH, 0))
	for _, line := range d.Body[start : start+l.bodyH] {
		rows = append(rows, row(line))
	}
	if l.gaps {
		rows = append(rows, row(""))
	}
	if l.footer {
		rows = append(rows, row(d.footer(t, panel, l, start)))
	}
	rows = append(rows, border.Render(b.BottomLeft+strings.Repeat(b.Bottom, max(l.outer-2, 0))+b.BottomRight))
	return strings.Join(rows, "\n")
}

// header is the bold title, truncated to leave room for the badge on the right.
func (d Dialog) header(t *Theme, panel lipgloss.Style, inner int, plain bool) string {
	badge := ""
	if d.Badge != "" {
		text := badgeText(d.Badge, plain)
		if ansi.StringWidth(text)+2 <= inner {
			badge = text
		}
	}
	titleW := inner
	if badge != "" {
		titleW = inner - ansi.StringWidth(badge) - 1
	}
	title := ansi.Truncate(d.Title, titleW, "…")
	out := panel.Foreground(t.Text).Bold(true).Render(title)
	if badge == "" {
		return out
	}
	gap := strings.Repeat(" ", max(inner-ansi.StringWidth(title)-ansi.StringWidth(badge), 1))
	style := lipgloss.NewStyle()
	if !plain {
		style = style.Background(t.Accent).Foreground(t.Surface).Bold(true)
	}
	return out + gap + style.Render(badge)
}

// footer is the dim hint, with a position indicator on the right when the
// body scrolls.
func (d Dialog) footer(t *Theme, panel lipgloss.Style, l dialogLayout, start int) string {
	dim := panel.Foreground(t.TextMuted)
	if len(d.Body) <= l.bodyH {
		return dim.Render(d.Footer)
	}
	ind := fmt.Sprintf("↑↓ %d-%d/%d", start+1, start+l.bodyH, len(d.Body))
	hintW := l.inner - ansi.StringWidth(ind) - 2
	if hintW < 4 {
		return dim.Render(ind)
	}
	hint := ansi.Truncate(d.Footer, hintW, "…")
	gap := strings.Repeat(" ", l.inner-ansi.StringWidth(hint)-ansi.StringWidth(ind))
	return dim.Render(hint) + gap + panel.Foreground(t.Accent).Render(ind)
}

// Overlay draws fg centered over bg on a w×h screen, line by line, keeping
// the background visible around it. A short background is padded to h
// rows; a dialog larger than the screen is clipped. w or h <= 0 fall back
// to the size of the content.
func Overlay(bg, fg string, w, h int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	fgW := 0
	for _, l := range fgLines {
		fgW = max(fgW, ansi.StringWidth(l))
	}
	if w <= 0 {
		w = fgW
		for _, l := range bgLines {
			w = max(w, ansi.StringWidth(l))
		}
	}
	if h <= 0 {
		h = max(len(bgLines), len(fgLines))
	}
	for len(bgLines) < h {
		bgLines = append(bgLines, "")
	}
	fgW = min(fgW, w)
	x := max((w-fgW)/2, 0)
	y := max((h-len(fgLines))/2, 0)
	const reset = "\x1b[0m"
	for i, fl := range fgLines {
		r := y + i
		if r >= h {
			break
		}
		bl := bgLines[r]
		left := ansi.Truncate(bl, x, "")
		if lw := ansi.StringWidth(left); lw < x {
			left += strings.Repeat(" ", x-lw)
		}
		fl = ansi.Truncate(fl, fgW, "")
		if fw := ansi.StringWidth(fl); fw < fgW {
			fl += strings.Repeat(" ", fgW-fw)
		}
		right := ""
		if bw := ansi.StringWidth(bl); bw > x+fgW {
			right = ansi.Cut(bl, x+fgW, min(bw, w))
		}
		bgLines[r] = left + reset + fl + reset + right
	}
	return strings.Join(bgLines, "\n")
}
