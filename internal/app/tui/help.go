package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesquel/termino/internal/tuikit"
)

// HelpModel is the help dialog: every key the TUI handles, grouped by
// context (tuikit.HelpSections), in two aligned columns. ↑↓/jk scroll it;
// any other key closes it.
type HelpModel struct {
	theme  *tuikit.Theme
	keys   tuikit.KeyMap
	width  int
	height int
	scroll int
}

// NewHelpModel creates a HelpModel.
func NewHelpModel(theme *tuikit.Theme, keys tuikit.KeyMap) HelpModel {
	return HelpModel{theme: theme, keys: keys}
}

// Reset scrolls back to the top, for a freshly opened help.
func (m HelpModel) Reset() HelpModel {
	m.scroll = 0
	return m
}

// Init implements tea.Model.
func (m HelpModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m HelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scroll = min(m.scroll, m.dialog().MaxScroll(m.width, m.height))
	case tea.KeyMsg:
		if scrollBurst(msg) {
			// Fast typing or key repeat delivers "jjj" as one message.
			var model tea.Model = m
			for _, r := range msg.Runes {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
			return model, nil
		}
		switch msg.String() {
		case "down", "j":
			m.scroll = min(m.scroll+1, m.dialog().MaxScroll(m.width, m.height))
		case "up", "k":
			m.scroll = max(m.scroll-1, 0)
		default:
			return m, dismissHelp()
		}
	}
	return m, nil
}

// scrollBurst reports whether msg is several j/k runes delivered at once.
func scrollBurst(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) < 2 || msg.Alt || msg.Paste {
		return false
	}
	return strings.Trim(string(msg.Runes), "jk") == ""
}

func dismissHelp() tea.Cmd {
	return func() tea.Msg { return tuikit.DismissHelpMsg{} }
}

// View implements tea.Model.
func (m HelpModel) View() string {
	if m.theme == nil {
		return ""
	}
	return m.dialog().Render(m.theme, m.width, m.height)
}

// dialog lays the sections out as rows: a section title, then one row per
// entry with the key column padded to a common width. Keys use the accent
// foreground only, so every cell keeps the panel background.
func (m HelpModel) dialog() tuikit.Dialog {
	sections := tuikit.HelpSections()
	keyW := 0
	for _, s := range sections {
		for _, e := range s.Entries {
			keyW = max(keyW, ansi.StringWidth(e.Label))
		}
	}
	var (
		head  = lipgloss.NewStyle()
		key   = lipgloss.NewStyle()
		desc  = lipgloss.NewStyle()
		lines []string
	)
	if m.theme != nil {
		head = head.Foreground(m.theme.Text).Bold(true)
		key = key.Foreground(m.theme.Accent)
		desc = desc.Foreground(m.theme.Text)
	}
	for i, s := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, head.Render(s.Title))
		for _, e := range s.Entries {
			label := e.Label + strings.Repeat(" ", keyW-ansi.StringWidth(e.Label))
			lines = append(lines, "  "+key.Render(label)+"  "+desc.Render(e.Desc))
		}
	}
	return tuikit.Dialog{
		Title:  "Keyboard Shortcuts",
		Badge:  "esc close",
		Body:   lines,
		Footer: "scroll ↑↓ · close esc or any other key",
		Scroll: m.scroll,
	}
}
