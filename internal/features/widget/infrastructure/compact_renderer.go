// Package infrastructure provides the widget renderers and SQLite repository.
package infrastructure

import (
	"strings"

	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// CompactRenderer renders the widget for narrow terminals (width < 60) in at
// most three lines: project and board hint, per-column counts, and the most
// relevant task (or the empty-state hint).
type CompactRenderer struct{}

// NewCompactRenderer returns a CompactRenderer.
func NewCompactRenderer() *CompactRenderer { return &CompactRenderer{} }

// Render formats summary into a compact string no wider than width.
func (r *CompactRenderer) Render(summary widgetdomain.Summary, width int) (string, error) {
	st := newStyles()
	head := header(summary)
	if len([]rune(head))+len(boardHint)+2 <= width {
		head = st.accent.Render(head) + "  " + st.muted.Render(boardHint)
	} else {
		head = st.accent.Render(clip(head, width))
	}
	lines := []string{head}
	if counts := countsLine(summary); counts != "" {
		lines = append(lines, st.muted.Render(clip(counts, width)))
	}

	secs := sections(summary)
	if hint := emptyState(summary, secs); hint != "" {
		lines = append(lines, st.title.Render(clip(hint, width)))
		return strings.Join(lines, "\n"), nil
	}
	top := secs[0]
	t := top.tasks[0]
	line := top.title + ": " + t.Ref + " " + t.Title
	if due := dueLabel(t.DueDate); due != "" {
		line += " · " + due
	}
	lines = append(lines, st.title.Render(clip(line, width)))
	return strings.Join(lines, "\n"), nil
}
