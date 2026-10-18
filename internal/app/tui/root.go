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
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
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
	screenSettings
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
	// Settings feeds the settings screen (key s on the board).
	Settings SettingsDeps
	// Share connects the board to repository sharing.
	Share ShareDeps
}

// ShareDeps connects the board to repository sharing; nil fields disable it.
type ShareDeps struct {
	// Version increases after every import of the shared files.
	Version func() int64
	// Warning returns the status-line warning for skipped shared files.
	Warning func() string
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
	settings SettingsModel

	actor taskapp.Actor // attribution for changes made in the TUI
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
		actor:     deps.MemberUC.LocalActor(context.Background(), deps.Config.UserName),
	}

	if deps.Config.CurrentProjectID == "" {
		m.screen = screenProjectSelector
		m.selector = m.newSelector()
	} else {
		m.screen = screenBoard
		m.board = m.newBoard(deps.Config.CurrentProjectID)
		m.activity = m.newActivity(deps.Config.CurrentProjectID)
	}
	m.help = NewHelpModel(th, keys)
	return m
}

// newSelector builds the project selector, marking the open project.
func (m RootModel) newSelector() projecttui.SelectorModel {
	return projecttui.NewSelectorModel(m.deps.ProjectUC, m.theme).WithCurrent(m.projectID)
}

// project looks up a project by ID; ok is false when it cannot be resolved.
func (m RootModel) project(id string) (projectdomain.Project, bool) {
	if m.deps.ProjectUC == nil || id == "" {
		return projectdomain.Project{}, false
	}
	projects, err := m.deps.ProjectUC.ListProjects(context.Background())
	if err != nil {
		return projectdomain.Project{}, false
	}
	for _, p := range projects {
		if p.ID == id {
			return p, true
		}
	}
	return projectdomain.Project{}, false
}

// newBoard builds the board model for projectID wired to the task, member
// and agent use cases. Nil use cases simply disable the matching feature.
func (m RootModel) newBoard(projectID string) boardtui.BoardModel {
	board := boardtui.NewBoardModel(projectID, m.deps.BoardUC, m.deps.DB, m.theme, m.keys).WithServices(m.boardServices())
	if p, ok := m.project(projectID); ok {
		board = board.WithProject(p.Name, p.Key)
	}
	return board
}

// boardServices wires the board to the task, member and agent use cases.
func (m RootModel) boardServices() boardtui.Services {
	svc := boardtui.Services{
		Actor:  m.actor,
		Motion: m.deps.Config.MotionEnabled(),
		OnDone: m.deps.OnTaskDone,

		ShareVersion: m.deps.Share.Version,
		ShareWarning: m.deps.Share.Warning,
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
	return svc
}

// newActivity builds the activity panel for projectID.
func (m RootModel) newActivity(projectID string) ActivityModel {
	act := NewActivityModel(projectID, activityLoader(m.deps.TaskUC), m.theme)
	if m.deps.TaskUC != nil {
		p, _ := m.project(projectID)
		act = act.WithTaskLabeler(taskLabeler(m.deps.TaskUC, p.Key))
	}
	act, _ = sized(act, m.sizeMsg())
	return act
}

// taskLabeler resolves a task ID to its "KEY-N" reference ("#N" when the
// project key is unknown).
func taskLabeler(uc *taskapp.TaskUseCases, projectKey string) TaskLabeler {
	return func(ctx context.Context, taskID string) string {
		t, err := uc.FindTaskByID(ctx, taskID)
		if err != nil {
			return ""
		}
		if projectKey != "" {
			return fmt.Sprintf("%s-%d", projectKey, t.Number)
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
		if msg.String() == "ctrl+c" {
			return m, tea.Quit // always quits, even from a prompt
		}
		// While the board has a prompt or picker open, every key is input.
		if m.screen == screenBoard && m.board.Capturing() {
			return m.routeToActiveScreen(msg)
		}
		// The new-task form and the detail editor are text input: q, p and ?
		// are typed, not global keys (esc leaves them).
		if m.screen == screenForm || (m.screen == screenTaskDetail && m.detail.Editing()) {
			return m.routeToActiveScreen(msg)
		}
		// The settings screen owns every key; esc leaves it.
		if m.screen == screenSettings {
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
				m.help = m.help.Reset()
				return m, nil
			}
		case "p":
			if m.screen != screenProjectSelector && m.screen != screenHelp {
				m.screen = screenProjectSelector
				m.selector = m.newSelector()
				m.selector, _ = sized(m.selector, m.sizeMsg())
				return m, m.selector.Init()
			}
		}

	case tuikit.DismissHelpMsg:
		m.screen = screenBoard
		return m, nil

	case SettingsClosedMsg:
		m.screen = screenBoard
		return m, nil

	case MotionChangedMsg:
		m.deps.Config.Motion = msg.On
		m.board = m.board.WithServices(m.boardServices())
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
		// Created reloads the board (Init would start a second polling ticker).
		var cmd tea.Cmd
		m.board, cmd = m.board.Created(msg.Task)
		return m, cmd
	}

	// Handle 'n' key on board to open form.
	if m.screen == screenBoard {
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "n" {
			colID := m.currentColumnID()
			actor := m.actor
			var creator tasktui.TaskCreator
			if m.deps.TaskUC != nil {
				creator = m.deps.TaskUC
			}
			m.form = tasktui.NewFormModel(
				m.projectID, colID,
				creator, actor,
				m.theme, m.keys,
			)
			m.form, _ = sized(m.form, m.sizeMsg())
			m.screen = screenForm
			return m, m.form.Init()
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "s" {
			p, _ := m.project(m.projectID)
			m.settings = NewSettingsModel(m.theme, m.deps.Config, m.deps.Settings, p.Name, p.Key).forProject(m.projectID)
			m.settings, _ = sized(m.settings, m.sizeMsg())
			m.screen = screenSettings
			return m, nil
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
			taskID := m.board.SelectedTaskID()
			if taskID != "" {
				actor := m.actor
				m.detail = tasktui.NewDetailModel(taskID, m.deps.TaskUC, actor, m.theme, m.keys).
					WithColumnNames(m.columnNames()).
					WithMemberNames(m.board.MemberNames()).
					WithTaskRef(m.board.TaskRef)
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
	case screenSettings:
		m.settings, cmd = sized(m.settings, size)
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
	case screenSettings:
		updated, cmd := m.settings.Update(msg)
		m.settings = updated.(SettingsModel)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model. Every screen other than the board is a dialog
// drawn over it, so the board stays visible around the dialog.
func (m RootModel) View() string {
	var dialog string
	switch m.screen {
	case screenBoard:
		return m.boardView()
	case screenProjectSelector:
		dialog = m.selector.View()
	case screenTaskDetail:
		dialog = m.detail.View()
	case screenForm:
		dialog = m.form.View()
	case screenHelp:
		dialog = m.help.View()
	case screenSettings:
		dialog = m.settings.View()
	default:
		return ""
	}
	return tuikit.Overlay(m.boardView(), dialog, m.width, m.height)
}

// boardView is the board with the activity panel, or "" before a project
// is open.
func (m RootModel) boardView() string {
	if m.projectID == "" {
		return ""
	}
	boardView := m.board.View()
	if m.activity.Visible() {
		return lipgloss.JoinHorizontal(lipgloss.Top, boardView, m.activity.View())
	}
	return boardView
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
