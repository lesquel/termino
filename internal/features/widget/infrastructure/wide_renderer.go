package infrastructure

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// WideRenderer renders the widget for wide terminals (width > 120).
// Output: project header + task table with ref, title, due-date, agent columns.
type WideRenderer struct{}

// NewWideRenderer returns a WideRenderer.
func NewWideRenderer() *WideRenderer { return &WideRenderer{} }

// Render formats summary with ref, title, due-date, and column name per task.
func (r *WideRenderer) Render(summary widgetdomain.Summary, width int) (string, error) {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7c3aed"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6b7280"))
	refStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e0e0e0"))
	dueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b"))
	colStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))

	var sb strings.Builder
	sb.WriteString(accent.Render(fmt.Sprintf("%s (%s)", summary.ProjectName, summary.ProjectKey)))
	sb.WriteString("\n")

	// Header row
	sb.WriteString(fmt.Sprintf("  %-10s  %-40s  %-12s  %s\n",
		headerStyle.Render("REF"),
		headerStyle.Render("TITLE"),
		headerStyle.Render("DUE"),
		headerStyle.Render("COLUMN"),
	))
	sb.WriteString("  " + strings.Repeat("─", width-4) + "\n")

	tasks := mergeUnique(summary.AssignedTasks, summary.InProgressTasks)
	if len(tasks) == 0 {
		sb.WriteString(refStyle.Render("  no active tasks"))
		return sb.String(), nil
	}

	maxTitle := width - 42
	if maxTitle < 10 {
		maxTitle = 10
	}
	if maxTitle > 40 {
		maxTitle = 40
	}

	for _, t := range tasks {
		title := t.Title
		if len(title) > maxTitle {
			title = title[:maxTitle-1] + "…"
		}

		due := "—"
		if t.DueDate != nil {
			days := int(time.Until(*t.DueDate).Hours() / 24)
			if days < 0 {
				due = dueStyle.Render("overdue")
			} else if days == 0 {
				due = dueStyle.Render("today")
			} else {
				due = dueStyle.Render(fmt.Sprintf("%dd", days))
			}
		}

		sb.WriteString(fmt.Sprintf("  %-10s  %-40s  %-12s  %s\n",
			refStyle.Render(t.Ref),
			titleStyle.Render(title),
			due,
			colStyle.Render(t.ColumnName),
		))
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
