// Package tui provides the TUI presentation layer for the project feature.
package tui

import (
	"context"
	"fmt"

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
}

func (i projectItem) Title() string       { return sanitize.Line(i.project.Name) }
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
	list   list.Model
	theme  *tuikit.Theme
	uc     *projectapp.ProjectUseCases
	width  int
	height int
	ready  bool
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
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(theme.Accent).
		Bold(true)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(theme.Accent)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(theme.Accent)

	return SelectorModel{
		list:  l,
		theme: theme,
		uc:    uc,
	}
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
		m.list.SetSize(msg.Width, msg.Height-4)

	case ProjectsLoadedMsg:
		if msg.Err != nil {
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.Projects))
		for _, p := range msg.Projects {
			items = append(items, projectItem{project: p})
		}
		m.list.SetItems(items)
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

// View implements tea.Model.
func (m SelectorModel) View() string {
	if !m.ready {
		return lipgloss.NewStyle().
			Foreground(m.theme.TextMuted).
			Render("Loading projects…")
	}
	if len(m.list.Items()) == 0 {
		return lipgloss.NewStyle().
			Foreground(m.theme.TextMuted).
			Render(fmt.Sprintf("No projects found.\n\nRun `termino project new` to create one."))
	}
	return m.list.View()
}
