package infrastructure

import (
	"strings"

	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// NormalRenderer renders the widget for medium terminals (60 <= width <= 120):
// project header, per-column counts, titled task lists and the board hint.
type NormalRenderer struct{}

// NewNormalRenderer returns a NormalRenderer.
func NewNormalRenderer() *NormalRenderer { return &NormalRenderer{} }

// Render formats summary with ref, title and (when set) due date per task.
func (r *NormalRenderer) Render(summary widgetdomain.Summary, width int) (string, error) {
	return renderList(summary, width, false), nil
}

// renderList draws the normal and wide layouts. wide adds aligned title,
// due-date and column fields; the due field is dropped when no listed task
// has a due date.
func renderList(s widgetdomain.Summary, width int, wide bool) string {
	st := newStyles()
	var lines []string
	lines = append(lines, st.accent.Render(clip(header(s), width)))
	if counts := countsLine(s); counts != "" {
		lines = append(lines, st.muted.Render(clip(counts, width)))
	}

	secs := sections(s)
	if hint := emptyState(s, secs); hint != "" {
		lines = append(lines, "  "+st.title.Render(hint))
		return strings.Join(lines, "\n")
	}

	refW := refWidth(secs)
	showDue := anyDue(secs)
	const dueW = 7 // "overdue"
	colW := 0
	if wide {
		for _, sec := range secs {
			for _, t := range sec.tasks {
				colW = max(colW, len([]rune(t.ColumnName)))
			}
		}
	}
	// Fixed cells: indent, ref, gaps, and the optional due/column fields.
	fixed := 2 + refW + 2
	if wide {
		fixed += 2 + colW
		if showDue {
			fixed += 2 + dueW
		}
	}
	titleW := max(width-fixed, 10)
	if wide {
		titleW = min(titleW, 50)
	}

	for _, sec := range secs {
		lines = append(lines, st.heading.Render(sec.title))
		for _, t := range sec.tasks {
			due := dueLabel(t.DueDate)
			var b strings.Builder
			b.WriteString("  " + st.muted.Render(pad(t.Ref, refW)) + "  ")
			if wide {
				b.WriteString(st.title.Render(pad(t.Title, titleW)))
				if showDue {
					if due == "" {
						due = "—"
					}
					b.WriteString("  " + st.due.Render(pad(due, dueW)))
				}
				b.WriteString("  " + st.column.Render(t.ColumnName))
			} else {
				title := t.Title
				if due != "" {
					title = clip(title, titleW-len(due)-3)
					b.WriteString(st.title.Render(title) + st.muted.Render(" · ") + st.due.Render(due))
				} else {
					b.WriteString(st.title.Render(clip(title, titleW)))
				}
			}
			lines = append(lines, strings.TrimRight(b.String(), " "))
		}
	}
	lines = append(lines, st.muted.Render(boardHint))
	return strings.Join(lines, "\n")
}
