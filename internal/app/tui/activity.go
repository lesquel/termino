package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
)

const (
	activityPanelWidth    = 36
	activityPanelMinWidth = 120
	maxActivityEntries    = 20
)

// ActivityLoadedMsg carries recently loaded activity entries.
type ActivityLoadedMsg struct {
	Entries []taskdomain.Activity
	Err     error
}

// ActivityLoader is a function that loads recent activity for a project.
type ActivityLoader func(ctx context.Context, projectID string, limit int) ([]taskdomain.Activity, error)

// ActivityModel renders the last 20 activity entries as a side panel.
// It is shown only when terminal width > activityPanelMinWidth.
type ActivityModel struct {
	entries   []taskdomain.Activity
	theme     *Theme
	loader    ActivityLoader
	projectID string
	width     int
	height    int
}

// NewActivityModel creates an ActivityModel.
func NewActivityModel(
	projectID string,
	loader ActivityLoader,
	theme *Theme,
) ActivityModel {
	return ActivityModel{
		projectID: projectID,
		loader:    loader,
		theme:     theme,
	}
}

// Init implements tea.Model.
func (m ActivityModel) Init() tea.Cmd {
	return m.loadActivityCmd()
}

func (m ActivityModel) loadActivityCmd() tea.Cmd {
	loader := m.loader
	projectID := m.projectID
	return func() tea.Msg {
		if loader == nil {
			return ActivityLoadedMsg{}
		}
		entries, err := loader(context.Background(), projectID, maxActivityEntries)
		return ActivityLoadedMsg{Entries: entries, Err: err}
	}
}

// Update implements tea.Model.
func (m ActivityModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case ActivityLoadedMsg:
		if msg.Err == nil {
			m.entries = msg.Entries
		}

	case RefreshMsg:
		// Share the same refresh tick as the board.
		return m, m.loadActivityCmd()
	}
	return m, nil
}

// Visible returns true when the terminal is wide enough for the panel.
func (m ActivityModel) Visible() bool {
	return m.width >= activityPanelMinWidth
}

// View implements tea.Model.
func (m ActivityModel) View() string {
	if !m.Visible() || m.theme == nil {
		return ""
	}
	return m.renderPanel()
}

func (m ActivityModel) renderPanel() string {
	panelStyle := lipgloss.NewStyle().
		Border(m.theme.BorderStyle).
		BorderForeground(m.theme.Border).
		Width(activityPanelWidth - 2).
		Padding(0, 1)

	headerStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent).
		Bold(true).
		MarginBottom(1)

	lines := []string{headerStyle.Render("Activity")}

	if len(m.entries) == 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(m.theme.TextMuted).
			Render("No recent activity"))
	} else {
		for _, e := range m.entries {
			lines = append(lines, m.renderEntry(e))
		}
	}
	return panelStyle.Render(strings.Join(lines, "\n"))
}

func (m ActivityModel) renderEntry(e taskdomain.Activity) string {
	actorStyle := lipgloss.NewStyle().Foreground(m.theme.Accent)
	actionStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	timeStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)

	actor := sanitize.Line(e.ActorID)
	if len(actor) > 10 {
		actor = actor[:10]
	}

	line := fmt.Sprintf("%s %s %s",
		actorStyle.Render(actor),
		actionStyle.Render(e.Action),
		timeStyle.Render(activityTimeAgo(e.CreatedAt)),
	)
	if len(line) > activityPanelWidth-4 {
		line = line[:activityPanelWidth-5] + "…"
	}
	return line
}

func activityTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
