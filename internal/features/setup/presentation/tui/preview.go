package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesquel/termino/internal/tuikit"
)

// previewCard is one sample card on the preview board.
type previewCard struct {
	n        string
	priority string // high, critical, low draw a symbol; done only colors the card
	title    string
}

var previewColumns = []struct {
	name  string
	cards []previewCard
}{
	{"Backlog", []previewCard{{"1", "high", "Fix login"}, {"2", "low", "Write docs"}}},
	{"In Progress", []previewCard{{"3", "critical", "Deploy"}}},
	{"Done", []previewCard{{"4", "done", "Setup"}}},
}

// RenderPreview draws a small sample board with theme t, at most width
// cells wide, so a theme can be judged before it is chosen.
func RenderPreview(t *tuikit.Theme, project, key string, width int) string {
	if project == "" {
		project = "My Project"
	}
	if key == "" {
		key = "KEY"
	}
	colW := (width-1)/3 - 2 // two border cells per column
	if colW > 18 {
		colW = 18
	}
	if colW < 8 {
		colW = 8
	}
	header := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
		Render(ansi.Truncate("termino · "+project+" ("+key+")", colW*3+6, "…"))

	cols := make([]string, len(previewColumns))
	for i, c := range previewColumns {
		lines := []string{lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(ansi.Truncate(c.name, colW, "…"))}
		for j, card := range c.cards {
			lines = append(lines, renderCard(t, key, card, colW, i == 0 && j == 0))
		}
		cols[i] = lipgloss.NewStyle().
			Border(t.BorderStyle).
			BorderForeground(t.Border).
			Width(colW).
			Height(3).
			Render(strings.Join(lines, "\n"))
	}
	footer := lipgloss.NewStyle().Foreground(t.TextMuted).Render(ansi.Truncate("n new · enter detail · ? help", colW*3+6, "…"))
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, cols...) + "\n" + footer
}

func renderCard(t *tuikit.Theme, key string, c previewCard, w int, selected bool) string {
	sym := t.Symbols[c.priority]
	if c.priority == "done" {
		sym = "" // board cards carry no done symbol
	}
	color := t.Text
	switch c.priority {
	case "critical", "high":
		color = t.Danger
	case "medium":
		color = t.Warning
	case "low":
		color = t.TextMuted
	case "done":
		color = t.Done
	}
	// Same layout as a board card: marker, priority symbol, KEY-N, title.
	marker := "  "
	if selected {
		marker = "> "
	}
	if sym != "" {
		sym += " "
	}
	line := ansi.Truncate(marker+sym+key+"-"+c.n+" "+c.title, w, "…")
	style := lipgloss.NewStyle().Foreground(color)
	if selected {
		style = style.Background(t.SurfaceAlt).Bold(true)
	}
	return style.Render(line)
}
