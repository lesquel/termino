package tui

import (
	"context"
	"database/sql"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	agentapp "github.com/lesquel/termino/internal/features/agent/application"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projecttui "github.com/lesquel/termino/internal/features/project/presentation/tui"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasktui "github.com/lesquel/termino/internal/features/task/presentation/tui"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/tuikit"
)

// screenKind identifies the active screen.
type screenKind int

const (
	screenProjectSelector screenKind = iota
	screenBoard
	screenTaskDetail
	screenHelp
	screenForm
)

// RootDeps holds all dependencies needed to build the root TUI model.
type RootDeps struct {
	Config    *config.Config
	DB        *sql.DB
	ProjectUC *projectapp.ProjectUseCases
	BoardUC   *boardapp.BoardUseCases
	TaskUC    *taskapp.TaskUseCases
	AgentUC   *agentapp.AgentUseCases
	MemberUC  *memberapp.MemberUseCases
	Theme     *tuikit.Theme
	// OnTaskDone, when set, runs after the board moves a task into the Done
	// column (used to check the task's checkbox in its source file).
	OnTaskDone func(taskID string)
}

// RootModel is the top-level Bubble Tea model.
// It owns screen routing and delegates Init/Update/View to active sub-models.
type RootModel struct {
	screen    screenKind
	theme     *tuikit.Theme
	keys      tuikit.KeyMap
	deps      RootDeps
	projectID string
	width     int
	height    int

	// Sub-models (lazily populated).
	selector projecttui.SelectorModel
	board    boardtui.BoardModel
	detail   tasktui.DetailModel
	form     tasktui.FormModel
	help     HelpModel
	activity ActivityModel
}

// NewRootModel constructs a RootModel and decides the initial screen.
func NewRootModel(deps RootDeps) RootModel {
	th := deps.Theme
	if th == nil {
		th = &tuikit.Theme{}
	}
	keys := tuikit.DefaultKeyMap()

	m := RootModel{
		theme:     th,
		keys:      keys,
		deps:      deps,
		projectID: deps.Config.CurrentProjectID,
	}

	if deps.Config.CurrentProjectID == "" {
		m.screen = screenProjectSelector
		m.selector = projecttui.NewSelectorModel(deps.ProjectUC, th)
	} else {
		m.screen = screenBoard
		m.board = m.newBoard(deps.Config.CurrentProjectID)
		m.activity = m.newActivity(deps.Config.CurrentProjectID)
	}
	m.help = NewHelpModel(th, keys)
	return m
}

// newBoard builds the board model for projectID wired to the task, member
// and agent use cases. Nil use cases simply disable the matching feature.
func (m RootModel) newBoard(projectID string) boardtui.BoardModel {
	svc := boardtui.Services{
		Actor:  defaultActor(m.deps.Config),
		Motion: m.deps.Config.MotionEnabled(),
		OnDone: m.deps.OnTaskDone,
	}
	if m.deps.TaskUC != nil {
		svc.Tasks = m.deps.TaskUC
	}
	if m.deps.MemberUC != nil {
		svc.Members = m.deps.MemberUC
	}
	if m.deps.AgentUC != nil {
		svc.Agents = m.deps.AgentUC
	}
	return boardtui.NewBoardModel(projectID, m.deps.BoardUC, m.deps.DB, m.theme, m.keys).WithServices(svc)
}

// newActivity builds the activity panel for projectID.
func (m RootModel) newActivity(projectID string) ActivityModel {
	act := NewActivityModel(projectID, activityLoader(m.deps.TaskUC), m.theme)
	if m.deps.TaskUC != nil {
		act = act.WithTaskLabeler(taskLabeler(m.deps.TaskUC))
	}
	act, _ = sized(act, m.sizeMsg())
	return act
}

// taskLabeler resolves a task ID to its "#N" reference.
func taskLabeler(uc *taskapp.TaskUseCases) TaskLabeler {
	return func(ctx context.Context, taskID string) string {
		t, err := uc.FindTaskByID(ctx, taskID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("#%d", t.Number)
	}
}

// activityLoader wraps the task use case's ListRecentActivity to satisfy ActivityLoader.
func activityLoader(uc *taskapp.TaskUseCases) ActivityLoader {
	if uc == nil {
		return func(ctx context.Context, projectID string, limit int) ([]taskdomain.Activity, error) {
			return nil, nil
		}
	}
	return func(ctx context.Context, projectID string, limit int) ([]taskdomain.Activity, error) {
		return uc.ListRecentActivity(ctx, projectID, limit)
	}
}

// Init implements tea.Model.
func (m RootModel) Init() tea.Cmd {
	switch m.screen {
	case screenProjectSelector:
		return m.selector.Init()
	case screenBoard:
		return tea.Batch(m.board.Init(), m.activity.Init())
	}
	return nil
}

// Update implements tea.Model.
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.resizeAll()

	case tea.KeyMsg:
		// While the board has a prompt or picker open, every key is input.
		if m.screen == screenBoard && m.board.Capturing() {
			return m.routeToActiveScreen(msg)
		}
		// While the selector is filtering, every key is typed into the filter.
		if m.screen == screenProjectSelector && m.selector.Filtering() {
			return m.routeToActiveScreen(msg)
		}
		// Global keys handled at root level.
		switch msg.String() {
		case "q":
			// q closes the project selector when there is a board to go back to.
			if m.screen == screenProjectSelector && m.projectID != "" {
				m.screen = screenBoard
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "?":
			if m.screen != screenHelp {
				m.screen = screenHelp
				return m, nil
			}
		case "p":
			if m.screen != screenProjectSelector && m.screen != screenHelp {
				m.screen = screenProjectSelector
				m.selector = projecttui.NewSelectorModel(m.deps.ProjectUC, m.theme)
				m.selector, _ = sized(m.selector, m.sizeMsg())
				return m, m.selector.Init()
			}
		}

	case tuikit.DismissHelpMsg:
		m.screen = screenBoard
		return m, nil

	case projecttui.ProjectSelectedMsg:
		m.projectID = msg.ProjectID
		m.screen = screenBoard
		m.board = m.newBoard(msg.ProjectID)
		m.activity = m.newActivity(msg.ProjectID)
		m.board, _ = sized(m.board, m.boardSizeMsg())
		return m, tea.Batch(m.board.Init(), m.activity.Init())

	case tasktui.GoBackMsg:
		m.screen = screenBoard
		// The task may have been edited in the detail view.
		return m, m.board.Reload()

	case tasktui.FormCancelledMsg:
		m.screen = screenBoard
		return m, nil

	case tasktui.TaskCreatedMsg:
		m.screen = screenBoard
		// Reload board (Init would start a second polling ticker).
		return m, m.board.Reload()
	}

	// Handle 'n' key on board to open form.
	if m.screen == screenBoard {
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "n" {
			colID := m.currentColumnID()
			actor := defaultActor(m.deps.Config)
			m.form = tasktui.NewFormModel(
				m.projectID, colID,
				m.deps.TaskUC, actor,
				m.theme, m.keys,
			)
			m.form, _ = sized(m.form, m.sizeMsg())
			m.screen = screenForm
			return m, m.form.Init()
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
			taskID := m.board.SelectedTaskID()
			if taskID != "" {
				actor := defaultActor(m.deps.Config)
				m.detail = tasktui.NewDetailModel(taskID, m.deps.TaskUC, actor, m.theme, m.keys).
					WithColumnNames(m.columnNames())
				m.detail, _ = sized(m.detail, m.sizeMsg())
				m.screen = screenTaskDetail
				return m, m.detail.Init()
			}
		}
	}

	return m.routeToActiveScreen(msg)
}

// sized feeds a size message to a freshly created sub-model so it never
// renders with a zero size before the next resize event.
func sized[T tea.Model](model T, msg tea.WindowSizeMsg) (T, tea.Cmd) {
	updated, cmd := model.Update(msg)
	if typed, ok := updated.(T); ok {
		return typed, cmd
	}
	return model, cmd
}

// sizeMsg is the last known terminal size.
func (m RootModel) sizeMsg() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: m.width, Height: m.height}
}

// boardSizeMsg is the area left for the board once the activity panel, when
// visible, takes its columns.
func (m RootModel) boardSizeMsg() tea.WindowSizeMsg {
	msg := m.sizeMsg()
	if activityVisibleAt(m.width) {
		msg.Width -= activityPanelWidth
	}
	return msg
}

// resizeAll propagates the terminal size to the persistent sub-models (board,
// activity, help) and to the active transient screen. Transient screens
// (selector, detail, form) are sized again whenever they are created.
func (m RootModel) resizeAll() (tea.Model, tea.Cmd) {
	size := m.sizeMsg()
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.board, cmd = sized(m.board, m.boardSizeMsg())
	cmds = append(cmds, cmd)
	m.activity, cmd = sized(m.activity, size)
	cmds = append(cmds, cmd)
	m.help, cmd = sized(m.help, size)
	cmds = append(cmds, cmd)
	switch m.screen {
	case screenProjectSelector:
		m.selector, cmd = sized(m.selector, size)
	case screenTaskDetail:
		m.detail, cmd = sized(m.detail, size)
	case screenForm:
		m.form, cmd = sized(m.form, size)
	default:
		cmd = nil
	}
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m RootModel) routeToActiveScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The board and activity panel keep live-polling behind overlays: their
	// self-rescheduling refresh tick and async results must reach them, or
	// the tick chain dies once any other screen is opened.
	var background tea.Cmd
	if m.screen != screenBoard && m.projectID != "" && !isInputMsg(msg) {
		updatedBoard, cmd := m.board.Update(msg)
		m.board = updatedBoard.(boardtui.BoardModel)
		updatedAct, cmd2 := m.activity.Update(msg)
		m.activity = updatedAct.(ActivityModel)
		background = tea.Batch(cmd, cmd2)
	}
	model, cmd := m.routeToScreen(msg)
	return model, tea.Batch(cmd, background)
}

// isInputMsg reports whether msg is user input, which only the active screen
// may receive.
func isInputMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		return true
	}
	return false
}

func (m RootModel) routeToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenProjectSelector:
		updated, cmd := m.selector.Update(msg)
		m.selector = updated.(projecttui.SelectorModel)
		return m, cmd
	case screenBoard:
		updatedBoard, cmd := m.board.Update(msg)
		m.board = updatedBoard.(boardtui.BoardModel)
		updatedAct, cmd2 := m.activity.Update(msg)
		m.activity = updatedAct.(ActivityModel)
		return m, tea.Batch(cmd, cmd2)
	case screenTaskDetail:
		updated, cmd := m.detail.Update(msg)
		m.detail = updated.(tasktui.DetailModel)
		return m, cmd
	case screenForm:
		updated, cmd := m.form.Update(msg)
		m.form = updated.(tasktui.FormModel)
		return m, cmd
	case screenHelp:
		updated, cmd := m.help.Update(msg)
		m.help = updated.(HelpModel)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m RootModel) View() string {
	switch m.screen {
	case screenProjectSelector:
		return m.selector.View()
	case screenBoard:
		boardView := m.board.View()
		if m.activity.Visible() {
			return lipgloss.JoinHorizontal(lipgloss.Top, boardView, m.activity.View())
		}
		return boardView
	case screenTaskDetail:
		return m.detail.View()
	case screenForm:
		formView := lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.form.View(),
		)
		return formView
	case screenHelp:
		m.help.width = m.width
		m.help.height = m.height
		return m.help.View()
	}
	return ""
}

// columnNames maps the board's column IDs to their names.
func (m RootModel) columnNames() map[string]string {
	names := make(map[string]string)
	for _, col := range m.board.Board().Columns {
		names[col.ID] = col.Name
	}
	return names
}

func (m RootModel) currentColumnID() string {
	cols := m.board.Board().Columns
	idx := m.board.ColIndex()
	if idx < len(cols) {
		return cols[idx].ID
	}
	return ""
}

// defaultActor builds a CLI actor from config.
func defaultActor(cfg *config.Config) taskapp.Actor {
	name := cfg.UserName
	if name == "" {
		name = "cli"
	}
	return taskapp.Actor{ID: "cli", Name: name, Kind: "human"}
}

// Start creates a tea.Program with alt-screen and runs it.
func Start(deps RootDeps) error {
	model := NewRootModel(deps)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
