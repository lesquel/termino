package infrastructure

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// NormalRenderer renders the widget for medium terminals (60 <= width <= 120).
// Output: project header + task list with ref and truncated title.
type NormalRenderer struct{}

// NewNormalRenderer returns a NormalRenderer.
func NewNormalRenderer() *NormalRenderer { return &NormalRenderer{} }

// Render formats summary with ref + title per task.
func (r *NormalRenderer) Render(summary widgetdomain.Summary, width int) (string, error) {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7c3aed"))
	refStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e0e0e0"))

	var sb strings.Builder
	sb.WriteString(accent.Render(fmt.Sprintf("%s (%s)", summary.ProjectName, summary.ProjectKey)))
	sb.WriteString("\n")

	maxTitle := width - 16
	if maxTitle < 10 {
		maxTitle = 10
	}

	tasks := mergeUnique(summary.AssignedTasks, summary.InProgressTasks)
	if len(tasks) == 0 {
		sb.WriteString(refStyle.Render("  no active tasks"))
		return sb.String(), nil
	}
	for _, t := range tasks {
		title := t.Title
		if len(title) > maxTitle {
			title = title[:maxTitle-1] + "…"
		}
		sb.WriteString(fmt.Sprintf("  %s  %s\n",
			refStyle.Render(t.Ref),
			titleStyle.Render(title),
		))
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// mergeUnique merges two slices, deduplicating by Ref.
func mergeUnique(a, b []widgetdomain.TaskSummary) []widgetdomain.TaskSummary {
	seen := make(map[string]struct{}, len(a)+len(b))
	result := make([]widgetdomain.TaskSummary, 0, len(a)+len(b))
	for _, t := range a {
		if _, ok := seen[t.Ref]; !ok {
			seen[t.Ref] = struct{}{}
			result = append(result, t)
		}
	}
	for _, t := range b {
		if _, ok := seen[t.Ref]; !ok {
			seen[t.Ref] = struct{}{}
			result = append(result, t)
		}
	}
	return result
}
