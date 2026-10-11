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
	Theme     *tuikit.Theme
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
		m.board = boardtui.NewBoardModel(
			deps.Config.CurrentProjectID,
			deps.BoardUC,
			deps.DB,
			th,
			keys,
		)
		m.activity = NewActivityModel(
			deps.Config.CurrentProjectID,
			activityLoader(deps.TaskUC),
			th,
		)
	}
	m.help = NewHelpModel(th, keys)
	return m
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
		return m.broadcast(msg)

	case tea.KeyMsg:
		// Global keys handled at root level.
		switch msg.String() {
		case "q", "ctrl+c":
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
				return m, m.selector.Init()
			}
		}

	case tuikit.DismissHelpMsg:
		m.screen = screenBoard
		return m, nil

	case projecttui.ProjectSelectedMsg:
		m.projectID = msg.ProjectID
		m.screen = screenBoard
		m.board = boardtui.NewBoardModel(
			msg.ProjectID,
			m.deps.BoardUC,
			m.deps.DB,
			m.theme,
			m.keys,
		)
		m.activity = NewActivityModel(
			msg.ProjectID,
			activityLoader(m.deps.TaskUC),
			m.theme,
		)
		return m, tea.Batch(m.board.Init(), m.activity.Init())

	case tasktui.GoBackMsg:
		m.screen = screenBoard
		return m, nil

	case tasktui.FormCancelledMsg:
		m.screen = screenBoard
		return m, nil

	case tasktui.TaskCreatedMsg:
		m.screen = screenBoard
		// Reload board.
		return m, m.board.Init()
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
			m.screen = screenForm
			return m, m.form.Init()
		}
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
			taskID := m.board.SelectedTaskID()
			if taskID != "" {
				actor := defaultActor(m.deps.Config)
				m.detail = tasktui.NewDetailModel(taskID, m.deps.TaskUC, actor, m.theme, m.keys)
				m.screen = screenTaskDetail
				return m, m.detail.Init()
			}
		}
	}

	return m.routeToActiveScreen(msg)
}

func (m RootModel) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch m.screen {
	case screenProjectSelector:
		updated, cmd := m.selector.Update(msg)
		m.selector = updated.(projecttui.SelectorModel)
		cmds = append(cmds, cmd)
	case screenBoard:
		updated, cmd := m.board.Update(msg)
		m.board = updated.(boardtui.BoardModel)
		cmds = append(cmds, cmd)
		updated2, cmd2 := m.activity.Update(msg)
		m.activity = updated2.(ActivityModel)
		cmds = append(cmds, cmd2)
	case screenTaskDetail:
		updated, cmd := m.detail.Update(msg)
		m.detail = updated.(tasktui.DetailModel)
		cmds = append(cmds, cmd)
	case screenForm:
		updated, cmd := m.form.Update(msg)
		m.form = updated.(tasktui.FormModel)
		cmds = append(cmds, cmd)
	case screenHelp:
		updated, cmd := m.help.Update(msg)
		m.help = updated.(HelpModel)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m RootModel) routeToActiveScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
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
