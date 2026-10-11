// Package infrastructure provides the widget renderers and SQLite repository.
package infrastructure

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// CompactRenderer renders the widget for narrow terminals (width < 60).
// Output: two lines — project name/key on the first, counts on the second.
type CompactRenderer struct{}

// NewCompactRenderer returns a CompactRenderer.
func NewCompactRenderer() *CompactRenderer { return &CompactRenderer{} }

// Render formats summary into a compact two-line string.
func (r *CompactRenderer) Render(summary widgetdomain.Summary, width int) (string, error) {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7c3aed"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	var sb strings.Builder
	header := accent.Render(fmt.Sprintf("%s (%s)", summary.ProjectName, summary.ProjectKey))
	sb.WriteString(header)
	sb.WriteString("\n")
	counts := muted.Render(fmt.Sprintf(
		"▸%d assigned  ●%d in-progress",
		len(summary.AssignedTasks),
		len(summary.InProgressTasks),
	))
	sb.WriteString(counts)
	return sb.String(), nil
}
