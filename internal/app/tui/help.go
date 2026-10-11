package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent).
		Bold(true).
		MarginBottom(1)

	keyStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent).
		Width(14)

	descStyle := lipgloss.NewStyle().
		Foreground(m.theme.Text)

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
		m.keys.Help,
		m.keys.Back,
		m.keys.Quit,
	}

	half := (len(bindings) + 1) / 2
	col1 := make([]string, 0, half)
	col2 := make([]string, 0, half)

	for i, b := range bindings {
		line := keyStyle.Render(b.Help().Key) + descStyle.Render(b.Help().Desc)
		if i < half {
			col1 = append(col1, line)
		} else {
			col2 = append(col2, line)
		}
	}

	colWidth := (m.width / 2) - 4
	leftCol := lipgloss.NewStyle().Width(colWidth).Render(strings.Join(col1, "\n"))
	rightCol := lipgloss.NewStyle().Width(colWidth).Render(strings.Join(col2, "\n"))
	cols := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)

	boxStyle := lipgloss.NewStyle().
		Border(m.theme.BorderStyle).
		BorderForeground(m.theme.Border).
		Background(m.theme.SurfaceAlt).
		Padding(1, 2)

	footer := lipgloss.NewStyle().
		Foreground(m.theme.TextMuted).
		MarginTop(1).
		Render("Press any key to dismiss")

	content := fmt.Sprintf("%s\n%s\n%s",
		titleStyle.Render("Keyboard Shortcuts"),
		cols,
		footer,
	)

	box := boxStyle.Render(content)

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}
