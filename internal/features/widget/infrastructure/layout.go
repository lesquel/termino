package infrastructure

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

const (
	// perSection caps the tasks listed under one heading.
	perSection = 3
	// emptyHint is shown for a project without any task.
	emptyHint = `No tasks yet — termino add "..."`
	// nothingOpenHint is shown when every task is done.
	nothingOpenHint = `Nothing open — termino add "..."`
	// boardHint tells the user how to open the full board.
	boardHint = "→ termino board"
)

// styles are the widget colours. lipgloss drops them when NO_COLOR is set
// or stdout is not a terminal.
type styles struct {
	accent, muted, heading, title, due, column lipgloss.Style
}

func newStyles() styles {
	return styles{
		accent:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7c3aed")),
		muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")),
		heading: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#a78bfa")),
		title:   lipgloss.NewStyle().Foreground(lipgloss.Color("#e0e0e0")),
		due:     lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b")),
		column:  lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")),
	}
}

// section is one titled task list of the widget.
type section struct {
	title string
	tasks []widgetdomain.TaskSummary
}

// sections orders the summary for display: in progress, my tasks, due soon,
// each without tasks already shown above; when none of these has anything,
// the next tasks of the first column.
func sections(s widgetdomain.Summary) []section {
	seen := map[string]bool{}
	pick := func(ts []widgetdomain.TaskSummary) []widgetdomain.TaskSummary {
		var out []widgetdomain.TaskSummary
		for _, t := range ts {
			if seen[t.Ref] || len(out) == perSection {
				continue
			}
			seen[t.Ref] = true
			out = append(out, t)
		}
		return out
	}
	var out []section
	add := func(title string, ts []widgetdomain.TaskSummary) {
		if picked := pick(ts); len(picked) > 0 {
			out = append(out, section{title: title, tasks: picked})
		}
	}
	add("In progress", s.InProgressTasks)
	add("My tasks", s.AssignedTasks)
	add("Due soon", s.DueSoonTasks)
	if len(out) == 0 && len(s.NextTasks) > 0 {
		title := "Up next"
		if col := s.NextTasks[0].ColumnName; col != "" {
			title += " in " + col
		}
		add(title, s.NextTasks)
	}
	return out
}

// header is "Project (KEY)".
func header(s widgetdomain.Summary) string {
	return fmt.Sprintf("%s (%s)", s.ProjectName, s.ProjectKey)
}

// countsLine is "Backlog 3 · In Progress 1 · Done 2".
func countsLine(s widgetdomain.Summary) string {
	parts := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		parts[i] = fmt.Sprintf("%s %d", c.Name, c.Count)
	}
	return strings.Join(parts, " · ")
}

// emptyState returns the hint for a summary with nothing to list, or "".
func emptyState(s widgetdomain.Summary, secs []section) string {
	switch {
	case s.TotalTasks() == 0:
		return emptyHint
	case len(secs) == 0:
		return nothingOpenHint
	}
	return ""
}

// dueLabel is a short relative due date: overdue, today or Nd.
func dueLabel(due *time.Time) string {
	if due == nil {
		return ""
	}
	d := time.Until(*due)
	switch {
	case d < 0:
		return "overdue"
	case d < 24*time.Hour:
		return "today"
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// anyDue reports whether a listed task has a due date.
func anyDue(secs []section) bool {
	for _, sec := range secs {
		for _, t := range sec.tasks {
			if t.DueDate != nil {
				return true
			}
		}
	}
	return false
}

// refWidth is the widest task reference listed.
func refWidth(secs []section) int {
	w := 0
	for _, sec := range secs {
		for _, t := range sec.tasks {
			w = max(w, ansi.StringWidth(t.Ref))
		}
	}
	return w
}

// pad fits s into exactly w cells: truncated with "…" or right-padded.
func pad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// clip truncates s to w cells (no-op when w <= 0).
func clip(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Truncate(s, w, "…")
}
