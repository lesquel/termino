package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	// Labels maps task IDs to display references ("#12"), when resolved.
	Labels map[string]string
	Err    error
}

// ActivityLoader is a function that loads recent activity for a project.
type ActivityLoader func(ctx context.Context, projectID string, limit int) ([]taskdomain.Activity, error)

// TaskLabeler resolves a task ID to a short display reference, or "".
type TaskLabeler func(ctx context.Context, taskID string) string

// ActivityModel renders the last 20 activity entries as a side panel.
// It is shown only when terminal width > activityPanelMinWidth.
type ActivityModel struct {
	entries   []taskdomain.Activity
	theme     *Theme
	loader    ActivityLoader
	labeler   TaskLabeler
	labels    map[string]string // task ID → reference, cached across reloads
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

// WithTaskLabeler returns a copy that shows a task reference on each entry.
func (m ActivityModel) WithTaskLabeler(l TaskLabeler) ActivityModel {
	m.labeler = l
	return m
}

// Init implements tea.Model.
func (m ActivityModel) Init() tea.Cmd {
	return m.loadActivityCmd()
}

func (m ActivityModel) loadActivityCmd() tea.Cmd {
	loader := m.loader
	labeler := m.labeler
	projectID := m.projectID
	known := m.labels
	return func() tea.Msg {
		if loader == nil {
			return ActivityLoadedMsg{}
		}
		ctx := context.Background()
		entries, err := loader(ctx, projectID, maxActivityEntries)
		if err != nil || labeler == nil {
			return ActivityLoadedMsg{Entries: entries, Err: err}
		}
		// Resolve only task IDs not seen before; references never change.
		labels := make(map[string]string)
		for _, e := range entries {
			if _, ok := known[e.TaskID]; ok {
				continue
			}
			if _, ok := labels[e.TaskID]; !ok {
				labels[e.TaskID] = labeler(ctx, e.TaskID)
			}
		}
		return ActivityLoadedMsg{Entries: entries, Labels: labels}
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
			if len(msg.Labels) > 0 {
				merged := make(map[string]string, len(m.labels)+len(msg.Labels))
				for k, v := range m.labels {
					merged[k] = v
				}
				for k, v := range msg.Labels {
					merged[k] = v
				}
				m.labels = merged
			}
		}

	case RefreshMsg:
		// Share the same refresh tick as the board.
		return m, m.loadActivityCmd()
	}
	return m, nil
}

// Visible returns true when the terminal is wide enough for the panel.
func (m ActivityModel) Visible() bool {
	return activityVisibleAt(m.width)
}

// activityVisibleAt reports whether a terminal of width columns shows the
// activity panel beside the board.
func activityVisibleAt(width int) bool {
	return width >= activityPanelMinWidth
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
		Width(activityPanelWidth-2).
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

// entryTextWidth is the room for one entry inside the panel border and padding.
const entryTextWidth = activityPanelWidth - 4

func (m ActivityModel) renderEntry(e taskdomain.Activity) string {
	actorStyle := lipgloss.NewStyle().Foreground(m.theme.Accent)
	actionStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	timeStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)

	// Truncate the plain text by display width first, then style each part,
	// so escape sequences are never cut and the panel never overflows.
	actor := ansi.Truncate(sanitize.Line(e.ActorID), 10, "…")
	action := sanitize.Line(e.Action)
	if ref := m.labels[e.TaskID]; ref != "" {
		action += " " + sanitize.Line(ref)
	}
	when := activityTimeAgo(e.CreatedAt)

	room := entryTextWidth - ansi.StringWidth(actor) - 1
	action = ansi.Truncate(action, max(room, 0), "…")
	room -= ansi.StringWidth(action) + 1
	if room < ansi.StringWidth(when) {
		when = ""
	}

	parts := []string{actorStyle.Render(actor), actionStyle.Render(action)}
	if when != "" {
		parts = append(parts, timeStyle.Render(when))
	}
	return strings.Join(parts, " ")
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
