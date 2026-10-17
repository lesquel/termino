package tui

import (
	"context"
	"fmt"
	"strings"

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

// TaskCreator is what the form needs to create a task and label it.
// *taskapp.TaskUseCases satisfies it.
type TaskCreator interface {
	CreateTask(ctx context.Context, actor taskapp.Actor, input taskapp.CreateTaskInput) (taskdomain.Task, error)
	AddLabels(ctx context.Context, actor taskapp.Actor, taskID string, names []string) error
}

// Only the focused input shows the "> " prompt, so focus is visible as text
// (NO_COLOR); the priority field shows ◀ ▶ arrows instead.
const (
	focusedPrompt = "> "
	blurredPrompt = "  "
	maxInputWidth = 60
)

// formField identifies the focused form field; tab cycles through them.
type formField int

const (
	fieldTitle formField = iota
	fieldPriority
	fieldLabels
	fieldCount
)

// priorities are offered in the priority field, lowest first.
var priorities = []taskdomain.Priority{
	taskdomain.PriorityLow,
	taskdomain.PriorityMedium,
	taskdomain.PriorityHigh,
	taskdomain.PriorityCritical,
}

// FormModel is the inline task-creation form triggered by pressing 'n' on the board.
type FormModel struct {
	titleInput  textinput.Model
	labelsInput textinput.Model
	priority    int // index into priorities
	focus       formField
	theme       *tuikit.Theme
	keys        tuikit.KeyMap
	uc          TaskCreator
	actor       taskapp.Actor
	projectID   string
	columnID    string
	width       int
	height      int
	err         string
}

// NewFormModel creates a FormModel for creating a task in the given column.
func NewFormModel(
	projectID, columnID string,
	uc TaskCreator,
	actor taskapp.Actor,
	theme *tuikit.Theme,
	keys tuikit.KeyMap,
) FormModel {
	ti := textinput.New()
	ti.Placeholder = "Task title (required)"
	ti.CharLimit = 200
	ti.Focus()

	li := textinput.New()
	li.Prompt = blurredPrompt
	li.Placeholder = "comma-separated, e.g. bug, ui (optional)"
	li.CharLimit = 200

	return FormModel{
		titleInput:  ti,
		labelsInput: li,
		priority:    1, // medium
		theme:       theme,
		keys:        keys,
		uc:          uc,
		actor:       actor,
		projectID:   projectID,
		columnID:    columnID,
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
		m.height = msg.Height
		// The prompt and the cursor take three cells of the dialog width.
		w := max(m.frame().Inner(m.width)-3, 1)
		m.titleInput.Width = w
		m.labelsInput.Width = w

	case TaskLoadedMsg:
		// A failed create comes back as an error-carrying load message.
		if msg.Err != nil {
			m.err = msg.Err.Error()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, cancelForm()
		case "enter":
			title := sanitize.Line(m.titleInput.Value())
			if title == "" {
				m.err = "Title is required"
				return m.focusOn(fieldTitle)
			}
			m.err = ""
			return m, m.submitForm()
		case "tab", "down":
			return m.focusOn((m.focus + 1) % fieldCount)
		case "shift+tab", "up":
			return m.focusOn((m.focus + fieldCount - 1) % fieldCount)
		}
		if m.focus == fieldPriority {
			switch msg.String() {
			case "left", "h", "-":
				m.priority = max(m.priority-1, 0)
			case "right", "l", "+", " ":
				m.priority = min(m.priority+1, len(priorities)-1)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	switch m.focus {
	case fieldTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case fieldLabels:
		m.labelsInput, cmd = m.labelsInput.Update(msg)
	}
	return m, cmd
}

// focusOn moves the keyboard focus to field f.
func (m FormModel) focusOn(f formField) (tea.Model, tea.Cmd) {
	m.focus = f
	m.titleInput.Blur()
	m.labelsInput.Blur()
	m.titleInput.Prompt = blurredPrompt
	m.labelsInput.Prompt = blurredPrompt
	switch f {
	case fieldTitle:
		m.titleInput.Prompt = focusedPrompt
		return m, m.titleInput.Focus()
	case fieldLabels:
		m.labelsInput.Prompt = focusedPrompt
		return m, m.labelsInput.Focus()
	}
	return m, nil
}

// labelNames splits the labels field on commas, dropping blanks.
func (m FormModel) labelNames() []string {
	var names []string
	for _, raw := range strings.Split(sanitize.Line(m.labelsInput.Value()), ",") {
		if name := strings.TrimSpace(raw); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (m FormModel) submitForm() tea.Cmd {
	title := sanitize.Line(m.titleInput.Value())
	actor := m.actor
	uc := m.uc
	labels := m.labelNames()
	input := taskapp.CreateTaskInput{
		ProjectID: m.projectID,
		Title:     title,
		ColumnID:  m.columnID,
		Priority:  priorities[m.priority],
	}
	return func() tea.Msg {
		if uc == nil {
			return TaskLoadedMsg{Err: fmt.Errorf("create task: tasks are not available")}
		}
		task, err := uc.CreateTask(context.Background(), actor, input)
		if err != nil {
			return TaskLoadedMsg{Err: fmt.Errorf("create task: %w", err)}
		}
		if len(labels) > 0 {
			if err := uc.AddLabels(context.Background(), actor, task.ID, labels); err != nil {
				return TaskLoadedMsg{Err: fmt.Errorf("label task: %w", err)}
			}
			task.Labels = labels
		}
		return TaskCreatedMsg{Task: task}
	}
}

func cancelForm() tea.Cmd {
	return func() tea.Msg { return FormCancelledMsg{} }
}

// frame is the form dialog without its body.
func (m FormModel) frame() tuikit.Dialog {
	return tuikit.Dialog{
		Title:  "New Task",
		Badge:  "esc cancel",
		Footer: "tab next field · ←/→ priority · enter create",
		Width:  maxInputWidth + 3,
	}
}

// View implements tea.Model. The form is a dialog; the root draws it over
// the board.
func (m FormModel) View() string {
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	errStyle := lipgloss.NewStyle().Foreground(m.theme.Danger)

	prio := string(priorities[m.priority])
	if m.focus == fieldPriority {
		prio = "◀ " + prio + " ▶"
	}

	d := m.frame()
	d.Body = []string{
		labelStyle.Render("Title:"),
		m.titleInput.View(),
		"",
		labelStyle.Render("Priority:") + " " + prio,
		"",
		labelStyle.Render("Labels:"),
		m.labelsInput.View(),
	}
	focusRow := map[formField]int{fieldTitle: 1, fieldPriority: 3, fieldLabels: 6}[m.focus]
	if m.err != "" {
		d.Body = append(d.Body, "", errStyle.Render(m.err))
		focusRow = len(d.Body) - 1
	}
	// Short terminals: drop the spacing rows, then scroll to the focused field.
	if room := d.BodyRoom(m.width, m.height); m.height > 0 && room < len(d.Body) {
		compact := make([]string, 0, len(d.Body))
		for i, l := range d.Body {
			if l == "" {
				if i < focusRow {
					focusRow--
				}
				continue
			}
			compact = append(compact, l)
		}
		d.Body = compact
		if room > 0 && focusRow >= room {
			d.Scroll = focusRow - room + 1
		}
	}
	return d.Render(m.theme, m.width, m.height)
}
