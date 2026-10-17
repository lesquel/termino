// Package tui provides the TUI presentation layer for the project feature.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/lesquel/termino/internal/tuikit"
)

// projectItem wraps a domain Project for the bubbles/list component.
type projectItem struct {
	project projectdomain.Project
	current bool
}

// Title marks the current project with text, so it survives NO_COLOR.
func (i projectItem) Title() string {
	if i.current {
		return sanitize.Line(i.project.Name) + "  (current)"
	}
	return sanitize.Line(i.project.Name)
}
func (i projectItem) Description() string { return i.project.Key }
func (i projectItem) FilterValue() string { return i.project.Name + " " + i.project.Key }

// ProjectsLoadedMsg carries the result of loading projects asynchronously.
type ProjectsLoadedMsg struct {
	Projects []projectdomain.Project
	Err      error
}

// ProjectSelectedMsg is sent when the user picks a project.
type ProjectSelectedMsg struct {
	ProjectID string
}

// SelectorModel is the Bubble Tea model for the project selection screen.
type SelectorModel struct {
	list    list.Model
	theme   *tuikit.Theme
	uc      *projectapp.ProjectUseCases
	current string // ID of the open project, "" when none
	width   int
	height  int
	ready   bool
}

// NewSelectorModel creates a new SelectorModel.
func NewSelectorModel(uc *projectapp.ProjectUseCases, theme *tuikit.Theme) SelectorModel {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(theme.Accent).
		BorderLeftForeground(theme.Accent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(theme.TextMuted).
		BorderLeftForeground(theme.Accent)

	l := list.New(nil, delegate, 0, 0)
	l.Title = "Select a Project"
	l.SetShowTitle(false) // the dialog header shows it
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(theme.Accent).
		Bold(true)
	// The root model owns ?, so the list's full-help toggle never fires.
	l.KeyMap.ShowFullHelp.SetEnabled(false)
	l.KeyMap.CloseFullHelp.SetEnabled(false)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(theme.Accent)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(theme.Accent)
	// The root model owns q; esc only clears a filter here, never quits.
	l.KeyMap.Quit = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))

	return SelectorModel{
		list:  l,
		theme: theme,
		uc:    uc,
	}
}

// WithCurrent marks the open project and makes q read "back", since q then
// returns to its board instead of quitting.
func (m SelectorModel) WithCurrent(projectID string) SelectorModel {
	m.current = projectID
	if projectID != "" {
		m.list.KeyMap.Quit.SetHelp("q", "back")
	}
	return m
}

// Init implements tea.Model.
func (m SelectorModel) Init() tea.Cmd {
	if m.uc == nil {
		return nil
	}
	return loadProjects(m.uc)
}

func loadProjects(uc *projectapp.ProjectUseCases) tea.Cmd {
	return func() tea.Msg {
		if uc == nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		projects, err := uc.ListProjects(context.Background())
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

// Update implements tea.Model.
func (m SelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		f := m.frame()
		room := f.BodyRoom(m.width, m.height)
		if m.height <= 0 {
			room = 20
		}
		m.list.SetSize(f.Inner(m.width), max(room, 1))

	case ProjectsLoadedMsg:
		if msg.Err != nil {
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.Projects))
		selected := 0
		for i, p := range msg.Projects {
			current := p.ID == m.current
			if current {
				selected = i
			}
			items = append(items, projectItem{project: p, current: current})
		}
		m.list.SetItems(items)
		m.list.Select(selected)
		m.ready = true

	case tea.KeyMsg:
		if msg.String() == "enter" && m.ready {
			if sel, ok := m.list.SelectedItem().(projectItem); ok {
				// Save selection to config.
				cfg, err := config.Load()
				if err == nil {
					cfg.CurrentProjectID = sel.project.ID
					_ = config.Save(cfg)
				}
				return m, func() tea.Msg {
					return ProjectSelectedMsg{ProjectID: sel.project.ID}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// frame is the selector dialog without its body.
func (m SelectorModel) frame() tuikit.Dialog {
	badge := "q quit"
	if m.current != "" {
		badge = "q back"
	}
	return tuikit.Dialog{Title: "Select a Project", Badge: badge, Width: 56}
}

// View implements tea.Model. The selector is a dialog; the root draws it
// over the board when one is open.
func (m SelectorModel) View() string {
	d := m.frame()
	muted := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	switch {
	case !m.ready:
		d.Body = []string{muted.Render("Loading projects…")}
	case len(m.list.Items()) == 0:
		d.Body = strings.Split(muted.Render("No projects yet.\n\nRun `termino setup` to create your first project,\nthen open the board again with `termino`.\n\nPress q to quit."), "\n")
	default:
		d.Body = strings.Split(m.list.View(), "\n")
	}
	return d.Render(m.theme, m.width, m.height)
}

// Filtering reports whether the list filter prompt is consuming keystrokes.
func (m SelectorModel) Filtering() bool {
	return m.list.FilterState() == list.Filtering
}
