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
		m.descTextarea.SetWidth(m.width - 8)

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
		m.scrollY++
	case "k", "up":
		if m.scrollY > 0 {
			m.scrollY--
		}
	case "e":
		m.editing = true
		m.editFocused = 0
		m.titleInput.Focus()
		return m, textinput.Blink
	case "esc":
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
	case "enter":
		if m.editFocused == 0 {
			// Submit edit.
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

// GoBackMsg signals a return to the previous screen.
type GoBackMsg struct{}

func goBack() tea.Cmd {
	return func() tea.Msg { return GoBackMsg{} }
}

// View implements tea.Model.
func (m DetailModel) View() string {
	if !m.ready {
		return lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("Loading task…")
	}

	if m.editing {
		return m.editView()
	}
	return m.detailView()
}

func (m DetailModel) detailView() string {
	t := m.task

	labelStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted).Width(14)
	valueStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	titleStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted).MarginTop(1)

	ref := fmt.Sprintf("#%d", t.Number)
	lines := []string{
		titleStyle.Render(ref + " " + sanitize.Line(t.Title)),
		"",
		labelStyle.Render("Priority:") + valueStyle.Render(string(t.Priority)),
		labelStyle.Render("Column:") + valueStyle.Render(t.ColumnID),
	}
	if t.AssigneeID != nil {
		lines = append(lines, labelStyle.Render("Assignee:")+valueStyle.Render(sanitize.Line(*t.AssigneeID)))
	}
	if t.DueDate != nil {
		lines = append(lines, labelStyle.Render("Due:")+valueStyle.Render(t.DueDate.Format("2006-01-02")))
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

	footer := lipgloss.NewStyle().Foreground(m.theme.TextMuted).
		Render("\ne=edit  ↑↓/jk=scroll  esc=back")

	content := strings.Join(lines, "\n")

	// Apply scroll.
	contentLines := strings.Split(content, "\n")
	maxScroll := len(contentLines) - m.height + 3
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollY > maxScroll {
		m.scrollY = maxScroll
	}
	start := m.scrollY
	end := start + m.height - 3
	if end > len(contentLines) {
		end = len(contentLines)
	}
	visible := strings.Join(contentLines[start:end], "\n")

	return visible + footer
}

func (m DetailModel) editView() string {
	titleStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)

	return fmt.Sprintf("%s\n\n%s\n%s\n\n%s\n%s\n\n%s",
		titleStyle.Render("Edit Task"),
		labelStyle.Render("Title:"),
		m.titleInput.View(),
		labelStyle.Render("Description:"),
		m.descTextarea.View(),
		lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("enter=save  tab=switch field  esc=cancel"),
	)
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
