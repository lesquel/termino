package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/lesquel/termino/internal/tuikit"
)

// TaskCreatedMsg is sent when a new task has been successfully created.
type TaskCreatedMsg struct {
	Task taskdomain.Task
}

// FormCancelledMsg is sent when the user cancels the form.
type FormCancelledMsg struct{}

// FormModel is the inline task-creation form triggered by pressing 'n' on the board.
type FormModel struct {
	titleInput textinput.Model
	theme      *tuikit.Theme
	keys       tuikit.KeyMap
	uc         *taskapp.TaskUseCases
	actor      taskapp.Actor
	projectID  string
	columnID   string
	width      int
	err        string
}

// NewFormModel creates a FormModel for creating a task in the given column.
func NewFormModel(
	projectID, columnID string,
	uc *taskapp.TaskUseCases,
	actor taskapp.Actor,
	theme *tuikit.Theme,
	keys tuikit.KeyMap,
) FormModel {
	ti := textinput.New()
	ti.Placeholder = "Task title (required)"
	ti.CharLimit = 200
	ti.Focus()

	return FormModel{
		titleInput: ti,
		theme:      theme,
		keys:       keys,
		uc:         uc,
		actor:      actor,
		projectID:  projectID,
		columnID:   columnID,
	}
}

// Init implements tea.Model.
func (m FormModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (m FormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.titleInput.Width = m.width - 8

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, cancelForm()
		case "enter":
			title := sanitize.Line(m.titleInput.Value())
			if title == "" {
				m.err = "Title is required"
				return m, nil
			}
			m.err = ""
			return m, m.submitForm()
		}
	}
	var cmd tea.Cmd
	m.titleInput, cmd = m.titleInput.Update(msg)
	return m, cmd
}

func (m FormModel) submitForm() tea.Cmd {
	title := sanitize.Line(m.titleInput.Value())
	actor := m.actor
	uc := m.uc
	input := taskapp.CreateTaskInput{
		ProjectID: m.projectID,
		Title:     title,
		ColumnID:  m.columnID,
		Priority:  taskdomain.PriorityMedium,
	}
	return func() tea.Msg {
		task, err := uc.CreateTask(context.Background(), actor, input)
		if err != nil {
			return TaskLoadedMsg{Err: fmt.Errorf("create task: %w", err)}
		}
		return TaskCreatedMsg{Task: task}
	}
}

func cancelForm() tea.Cmd {
	return func() tea.Msg { return FormCancelledMsg{} }
}

// View implements tea.Model.
func (m FormModel) View() string {
	titleStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	errStyle := lipgloss.NewStyle().Foreground(m.theme.Danger)

	errLine := ""
	if m.err != "" {
		errLine = "\n" + errStyle.Render(m.err)
	}

	boxStyle := lipgloss.NewStyle().
		Border(m.theme.BorderStyle).
		BorderForeground(m.theme.Border).
		Padding(1, 2)

	content := fmt.Sprintf("%s\n\n%s\n%s%s\n\n%s",
		titleStyle.Render("New Task"),
		labelStyle.Render("Title:"),
		m.titleInput.View(),
		errLine,
		labelStyle.Render("enter=create  esc=cancel"),
	)
	return boxStyle.Render(content)
}
