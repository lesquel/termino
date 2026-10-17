// Package tui provides the TUI presentation layer for the task feature.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/lesquel/termino/internal/tuikit"
)

// TaskLoadedMsg carries the full task data loaded asynchronously.
type TaskLoadedMsg struct {
	Task     taskdomain.Task
	Comments []taskdomain.Comment
	Activity []taskdomain.Activity
	Err      error
}

// TaskUpdatedMsg signals that an edit was saved successfully.
type TaskUpdatedMsg struct{}

// DetailModel is the Bubble Tea model for the task detail screen.
type DetailModel struct {
	task     taskdomain.Task
	comments []taskdomain.Comment
	activity []taskdomain.Activity
	theme    *tuikit.Theme
	keys     tuikit.KeyMap
	uc       *taskapp.TaskUseCases
	actor    taskapp.Actor
	taskID   string
	scrollY  int
	width    int
	height   int
	ready    bool
	editing  bool
	columns  map[string]string // column ID → name, for display
	members  map[string]string // member ID → name, for display
	ref      func(n int) string
	flash    string // one-shot feedback such as "Saved", cleared by the next key

	// Edit form fields.
	titleInput   textinput.Model
	descTextarea textarea.Model
	editFocused  int // 0=title, 1=desc
}

// NewDetailModel creates a DetailModel for the given task ID.
func NewDetailModel(
	taskID string,
	uc *taskapp.TaskUseCases,
	actor taskapp.Actor,
	theme *tuikit.Theme,
	keys tuikit.KeyMap,
) DetailModel {
	ti := textinput.New()
	ti.Placeholder = "Task title"
	ti.CharLimit = 200

	ta := textarea.New()
	ta.Placeholder = "Task description (optional)"
	ta.SetWidth(60)
	ta.SetHeight(5)

	return DetailModel{
		taskID:       taskID,
		uc:           uc,
		actor:        actor,
		theme:        theme,
		keys:         keys,
		titleInput:   ti,
		descTextarea: ta,
	}
}

// WithColumnNames sets the column names shown instead of column IDs.
func (m DetailModel) WithColumnNames(names map[string]string) DetailModel {
	m.columns = names
	return m
}

// WithMemberNames sets the member names shown instead of member IDs.
func (m DetailModel) WithMemberNames(names map[string]string) DetailModel {
	m.members = names
	return m
}

// WithTaskRef sets how task numbers are displayed (e.g. KEY-N).
func (m DetailModel) WithTaskRef(ref func(n int) string) DetailModel {
	m.ref = ref
	return m
}

// taskRef returns the display reference of the loaded task.
func (m DetailModel) taskRef() string {
	if m.ref != nil {
		return m.ref(m.task.Number)
	}
	return fmt.Sprintf("#%d", m.task.Number)
}

// memberName returns the display name of a member, falling back to its ID.
func (m DetailModel) memberName(id string) string {
	if name, ok := m.members[id]; ok && name != "" {
		return name
	}
	return id
}

// columnName returns the display name of a column, falling back to its ID.
func (m DetailModel) columnName(id string) string {
	if name, ok := m.columns[id]; ok && name != "" {
		return name
	}
	return id
}

// Init implements tea.Model.
func (m DetailModel) Init() tea.Cmd {
	return loadTask(m.uc, m.taskID)
}

func loadTask(uc *taskapp.TaskUseCases, taskID string) tea.Cmd {
	return func() tea.Msg {
		t, err := uc.FindTaskByID(context.Background(), taskID)
		if err != nil {
			return TaskLoadedMsg{Err: err}
		}
		comments, _ := uc.ListCommentsByTask(context.Background(), taskID)
		activity, _ := uc.ListActivityByTask(context.Background(), taskID)
		return TaskLoadedMsg{Task: t, Comments: comments, Activity: activity}
	}
}

func loadTaskComments(uc *taskapp.TaskUseCases, taskID string) ([]taskdomain.Comment, error) {
	return uc.ListCommentsByTask(context.Background(), taskID)
}

func loadTaskActivity(uc *taskapp.TaskUseCases, taskID string) ([]taskdomain.Activity, error) {
	return uc.ListActivityByTask(context.Background(), taskID)
}

// Update implements tea.Model.
func (m DetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inner := m.frame().Inner(m.width)
		m.titleInput.Width = max(inner-3, 1) // the prompt takes cells too
		m.descTextarea.SetWidth(max(inner, 1))
		m.descTextarea.SetHeight(min(5, max(m.height-14, 1)))
		m.scrollY = min(m.scrollY, m.maxScroll())

	case TaskUpdatedMsg:
		m.flash = "Saved"
		m.editing = false
		m.titleInput.Blur()
		m.descTextarea.Blur()
		return m, loadTask(m.uc, m.taskID)

	case TaskLoadedMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.task = msg.Task
		m.comments = msg.Comments
		m.activity = msg.Activity
		m.ready = true
		m.titleInput.SetValue(sanitize.Line(m.task.Title))
		m.descTextarea.SetValue(sanitize.Clean(m.task.Description))

	case tea.KeyMsg:
		m.flash = ""
		if m.editing {
			return m.handleEditKey(msg)
		}
		return m.handleViewKey(msg)
	}

	if m.editing {
		var cmd1, cmd2 tea.Cmd
		m.titleInput, cmd1 = m.titleInput.Update(msg)
		m.descTextarea, cmd2 = m.descTextarea.Update(msg)
		return m, tea.Batch(cmd1, cmd2)
	}
	return m, nil
}

func (m DetailModel) handleViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.scrollY < m.maxScroll() {
			m.scrollY++
		}
	case "k", "up":
		if m.scrollY > 0 {
			m.scrollY--
		}
	case "e":
		m.editing = true
		m.editFocused = 0
		m.titleInput.Focus()
		return m, textinput.Blink
	case "esc", "enter":
		return m, goBack()
	}
	return m, nil
}

func (m DetailModel) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = false
		m.titleInput.Blur()
		m.descTextarea.Blur()
		return m, nil
	case "tab":
		m.editFocused = (m.editFocused + 1) % 2
		if m.editFocused == 0 {
			m.titleInput.Focus()
			m.descTextarea.Blur()
			return m, textinput.Blink
		}
		m.titleInput.Blur()
		m.descTextarea.Focus()
		return m, textarea.Blink
	case "ctrl+s":
		return m, m.saveEdit()
	case "enter":
		// Enter saves from the title; in the description it inserts a newline.
		if m.editFocused == 0 {
			return m, m.saveEdit()
		}
	}
	var cmd1, cmd2 tea.Cmd
	m.titleInput, cmd1 = m.titleInput.Update(msg)
	m.descTextarea, cmd2 = m.descTextarea.Update(msg)
	return m, tea.Batch(cmd1, cmd2)
}

func (m DetailModel) saveEdit() tea.Cmd {
	title := sanitize.Line(m.titleInput.Value())
	desc := sanitize.Clean(m.descTextarea.Value())
	if title == "" {
		title = sanitize.Line(m.task.Title) // never blank a title through an edit
	}
	actor := m.actor
	uc := m.uc
	taskID := m.task.ID
	return func() tea.Msg {
		if err := uc.UpdateTask(context.Background(), actor, taskapp.UpdateTaskInput{
			TaskID:      taskID,
			Title:       &title,
			Description: &desc,
		}); err != nil {
			return TaskLoadedMsg{Err: err}
		}
		return TaskUpdatedMsg{}
	}
}

// Editing reports whether the edit form is open and consuming keystrokes.
func (m DetailModel) Editing() bool { return m.editing }

// GoBackMsg signals a return to the previous screen.
type GoBackMsg struct{}

func goBack() tea.Cmd {
	return func() tea.Msg { return GoBackMsg{} }
}

// detailWidth is the preferred inner width of the detail dialog.
const detailWidth = 64

// View implements tea.Model. The detail is a dialog; the root draws it over
// the board.
func (m DetailModel) View() string {
	return m.dialog().Render(m.theme, m.width, m.height)
}

// frame is the dialog without its body, enough to size the content.
func (m DetailModel) frame() tuikit.Dialog {
	if !m.ready {
		return tuikit.Dialog{Title: "Task", Badge: "esc close", Width: detailWidth}
	}
	if m.editing {
		hint := "enter/ctrl+s=save · tab=switch field · esc=cancel"
		if m.editFocused == 1 {
			hint = "ctrl+s=save · enter=newline · tab=switch field · esc=cancel"
		}
		return tuikit.Dialog{
			Title:    "Edit Task",
			Subtitle: m.taskRef() + " " + sanitize.Line(m.task.Title),
			Badge:    "esc cancel",
			Footer:   hint,
			Width:    detailWidth,
		}
	}
	footer := "scroll ↑↓ · edit e · close esc / enter"
	if m.flash != "" {
		footer = "✓ " + m.flash + " · " + footer
	}
	return tuikit.Dialog{
		Title:    m.taskRef() + " " + sanitize.Line(m.task.Title),
		Subtitle: sanitize.Line(m.columnName(m.task.ColumnID)),
		Badge:    "esc close",
		Footer:   footer,
		Width:    detailWidth,
	}
}

// dialog is the frame with the body for the current state.
func (m DetailModel) dialog() tuikit.Dialog {
	d := m.frame()
	switch {
	case !m.ready:
		d.Body = []string{lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("Loading task…")}
	case m.editing:
		d.Body = m.editLines()
		if m.editFocused == 1 {
			d.Scroll = len(d.Body) // clamped: keeps the description in view
		}
	default:
		d.Body = m.contentLines(d.Inner(m.width))
		d.Scroll = m.scrollY
	}
	return d
}

// maxScroll is the largest valid scroll offset for the current content.
func (m DetailModel) maxScroll() int {
	if !m.ready || m.editing {
		return 0
	}
	return m.dialog().MaxScroll(m.width, m.height)
}

// contentLines is the scrollable body: fields, description, comments and
// activity, wrapped to width cells.
func (m DetailModel) contentLines(width int) []string {
	t := m.task

	labelStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted).Width(12)
	valueStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	sectionStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)

	field := func(label, value string) string {
		return labelStyle.Render(label) + valueStyle.Render(value)
	}
	assignee := "unassigned"
	if t.AssigneeID != nil {
		assignee = sanitize.Line(m.memberName(*t.AssigneeID))
	}
	lines := []string{
		field("Priority:", string(t.Priority)),
		field("Assignee:", assignee),
	}
	if len(t.Labels) > 0 {
		labels := make([]string, len(t.Labels))
		for i, l := range t.Labels {
			labels[i] = sanitize.Line(l)
		}
		lines = append(lines, field("Labels:", strings.Join(labels, ", ")))
	}
	if t.DueDate != nil {
		lines = append(lines, field("Due:", t.DueDate.Format("2006-01-02")))
	}
	if t.Description != "" {
		lines = append(lines, "", sectionStyle.Render("Description"), sanitize.Clean(t.Description))
	}

	// Comments.
	if len(m.comments) > 0 {
		lines = append(lines, "", sectionStyle.Render("Comments"))
		for _, c := range m.comments {
			lines = append(lines, fmt.Sprintf("  %s  %s", sanitize.Line(c.AuthorID), sanitize.Line(c.Body)))
		}
	}

	// Activity.
	if len(m.activity) > 0 {
		lines = append(lines, "", sectionStyle.Render("Activity"))
		for _, a := range m.activity {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s",
				sanitize.Line(a.ActorID), a.Action, timeAgo(a.CreatedAt)))
		}
	}

	// Wrap long rows so nothing is cut at the dialog edge.
	text := strings.Join(lines, "\n")
	if width > 0 {
		text = ansi.Wrap(text, width, " ")
	}
	return strings.Split(text, "\n")
}

// editLines is the body of the edit form.
func (m DetailModel) editLines() []string {
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	lines := []string{labelStyle.Render("Title:"), m.titleInput.View(), "", labelStyle.Render("Description:")}
	return append(lines, strings.Split(m.descTextarea.View(), "\n")...)
}

func timeAgo(t time.Time) string {
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
