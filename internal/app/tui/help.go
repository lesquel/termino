package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesquel/termino/internal/tuikit"
)

// HelpModel is a full-screen help overlay that lists all key bindings.
type HelpModel struct {
	theme  *tuikit.Theme
	keys   tuikit.KeyMap
	width  int
	height int
}

// NewHelpModel creates a HelpModel.
func NewHelpModel(theme *tuikit.Theme, keys tuikit.KeyMap) HelpModel {
	return HelpModel{theme: theme, keys: keys}
}

// Init implements tea.Model.
func (m HelpModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m HelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		_ = msg
		return m, dismissHelp()
	}
	return m, nil
}

func dismissHelp() tea.Cmd {
	return func() tea.Msg { return tuikit.DismissHelpMsg{} }
}

// View implements tea.Model.
func (m HelpModel) View() string {
	if m.theme == nil {
		return ""
	}

	bindings := []key.Binding{
		m.keys.Up,
		m.keys.Down,
		m.keys.Left,
		m.keys.Right,
		m.keys.Enter,
		m.keys.NewTask,
		m.keys.Edit,
		m.keys.Delete,
		m.keys.Move,
		m.keys.Assign,
		m.keys.Filter,
		m.keys.ProjectSelect,
		m.keys.Settings,
		m.keys.Help,
		m.keys.Back,
		m.keys.Quit,
	}

	keyWidth := 0
	for _, b := range bindings {
		keyWidth = max(keyWidth, ansi.StringWidth(b.Help().Key))
	}
	keyWidth += 2
	keyStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Width(keyWidth)
	descStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	lines := make([]string, len(bindings))
	for i, b := range bindings {
		lines[i] = keyStyle.Render(b.Help().Key) + descStyle.Render(b.Help().Desc)
	}

	// Two columns when they fit, otherwise one with a compact frame. Lines
	// are never wrapped, so a key and its description stay together.
	const gap = "   "
	half := (len(lines) + 1) / 2
	left := strings.Join(lines[:half], "\n")
	right := strings.Join(lines[half:], "\n")
	frameW := 2 + 4 // border + horizontal padding
	twoCols := lipgloss.Width(left)+len(gap)+lipgloss.Width(right)+frameW <= m.width
	cols := strings.Join(lines, "\n")
	if twoCols {
		cols = lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
	}
	compact := !twoCols && len(lines)+8 > m.height

	titleStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	footerStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	boxStyle := lipgloss.NewStyle().
		Border(m.theme.BorderStyle).
		BorderForeground(m.theme.Border).
		Background(m.theme.SurfaceAlt)
	if compact {
		boxStyle = boxStyle.Padding(0, 1)
	} else {
		boxStyle = boxStyle.Padding(1, 2)
		titleStyle = titleStyle.MarginBottom(1)
		footerStyle = footerStyle.MarginTop(1)
	}

	content := fmt.Sprintf("%s\n%s\n%s",
		titleStyle.Render("Keyboard Shortcuts"),
		cols,
		footerStyle.Render("Press any key to dismiss"),
	)
	if inner := m.width - 4; inner > 0 {
		// Last resort on very narrow terminals: cut, never wrap.
		cut := strings.Split(content, "\n")
		for i, l := range cut {
			cut[i] = ansi.Truncate(l, inner, "…")
		}
		content = strings.Join(cut, "\n")
	}
	box := boxStyle.Render(content)

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}
