// Package tuikit contains shared TUI types (Theme, KeyMap, message types)
// used across all presentation layers. It has no imports from other internal
// packages to avoid circular dependencies.
package tuikit

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Theme holds semantic color tokens for all TUI components.
// Never use hardcoded color strings — always reference Theme fields.
type Theme struct {
	Surface    lipgloss.Color
	SurfaceAlt lipgloss.Color
	Text       lipgloss.Color
	TextMuted  lipgloss.Color
	Accent     lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Danger     lipgloss.Color
	Border     lipgloss.Color
	Done       lipgloss.Color

	// BorderStyle is the lipgloss border used for panels and cards.
	BorderStyle lipgloss.Border
	// Symbols maps semantic names (done, high, critical, medium, low) to display characters.
	Symbols map[string]string
}

// KeyMap defines all keyboard bindings used by the TUI.
type KeyMap struct {
	Up            key.Binding
	Down          key.Binding
	Left          key.Binding
	Right         key.Binding
	Enter         key.Binding
	NewTask       key.Binding
	Edit          key.Binding
	Delete        key.Binding
	Move          key.Binding
	Assign        key.Binding
	Filter        key.Binding
	ProjectSelect key.Binding
	Quit          key.Binding
	Help          key.Binding
	Back          key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left column"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right column"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "open detail"),
		),
		NewTask: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new task"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit (in task detail)"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete (asks y/N)"),
		),
		Move: key.NewBinding(
			key.WithKeys("m", "H", "L"),
			key.WithHelp("m/H/L", "move: picker / left / right"),
		),
		Assign: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "assign"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter: text, assignee: label: priority: agent:"),
		),
		ProjectSelect: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "project selector"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}

// RefreshMsg is dispatched by the live-polling tick to trigger a board reload.
type RefreshMsg struct{}

// DismissHelpMsg is sent when the user presses any key on the help screen.
type DismissHelpMsg struct{}
