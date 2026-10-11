// Package tui provides the TUI presentation layer for the board feature.
package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/tuikit"
)

const (
	pollInterval   = 500 * time.Millisecond
	minBoardWidth  = 80
	minBoardHeight = 24
	columnMinWidth = 20
)

// BoardLoadedMsg carries a refreshed board from an async load.
type BoardLoadedMsg struct {
	Board boarddomain.Board
	Tasks map[string][]taskdomain.Task // colID → tasks
	Err   error
}

// RefreshMsg triggers a board reload when the data_version pragma changes.
// Re-exported here as an alias from tuikit.
type RefreshMsg = tuikit.RefreshMsg

// dataVersionMsg carries the current SQLite data_version.
type dataVersionMsg struct {
	version int
	err     error
}

// BoardModel is the Bubble Tea model for the Kanban board view.
type BoardModel struct {
	board     boarddomain.Board
	tasks     map[string][]taskdomain.Task // colID → ordered tasks
	colIndex  int                          // selected column index
	taskIndex int                          // selected task index within column
	theme     *tuikit.Theme
	keys      tuikit.KeyMap
	boardUC   *boardapp.BoardUseCases
	db        *sql.DB // for live polling (read-only connection)
	projectID string
	width     int
	height    int
	ready     bool
	dataVer   int
}

// NewBoardModel creates a new BoardModel for the given project.
func NewBoardModel(
	projectID string,
	boardUC *boardapp.BoardUseCases,
	db *sql.DB,
	theme *tuikit.Theme,
	keys tuikit.KeyMap,
) BoardModel {
	return BoardModel{
		projectID: projectID,
		boardUC:   boardUC,
		db:        db,
		theme:     theme,
		keys:      keys,
		tasks:     make(map[string][]taskdomain.Task),
	}
}

// Init implements tea.Model — starts loading the board and the polling tick.
func (m BoardModel) Init() tea.Cmd {
	return tea.Batch(
		loadBoard(m.boardUC, m.projectID),
		tickPoll(),
	)
}

func loadBoard(uc *boardapp.BoardUseCases, projectID string) tea.Cmd {
	return func() tea.Msg {
		board, err := uc.GetBoard(context.Background(), projectID)
		if err != nil {
			return BoardLoadedMsg{Err: err}
		}
		return BoardLoadedMsg{Board: board, Err: nil}
	}
}

func tickPoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return tuikit.RefreshMsg{}
	})
}

// Update implements tea.Model.
func (m BoardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case BoardLoadedMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.board = msg.Board
		if msg.Tasks != nil {
			m.tasks = msg.Tasks
		}
		m.ready = true
		m.clampSelection()

	case tuikit.RefreshMsg:
		return m, tea.Batch(
			checkDataVersion(m.db, m.dataVer),
			tickPoll(),
		)

	case dataVersionMsg:
		if msg.err != nil {
			return m, nil
		}
		if msg.version != m.dataVer {
			m.dataVer = msg.version
			return m, loadBoard(m.boardUC, m.projectID)
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *BoardModel) clampSelection() {
	cols := m.board.Columns
	if len(cols) == 0 {
		return
	}
	if m.colIndex >= len(cols) {
		m.colIndex = len(cols) - 1
	}
	colID := cols[m.colIndex].ID
	tasks := m.board.Tasks[colID]
	if m.taskIndex >= len(tasks) {
		if len(tasks) > 0 {
			m.taskIndex = len(tasks) - 1
		} else {
			m.taskIndex = 0
		}
	}
}

func (m BoardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cols := m.board.Columns
	if len(cols) == 0 {
		return m, nil
	}
	switch msg.String() {
	case "left", "h":
		if m.colIndex > 0 {
			m.colIndex--
			m.taskIndex = 0
		}
	case "right", "l":
		if m.colIndex < len(cols)-1 {
			m.colIndex++
			m.taskIndex = 0
		}
	case "up", "k":
		if m.taskIndex > 0 {
			m.taskIndex--
		}
	case "down", "j":
		colID := cols[m.colIndex].ID
		max := len(m.board.Tasks[colID]) - 1
		if m.taskIndex < max {
			m.taskIndex++
		}
	}
	return m, nil
}

// SelectedTaskID returns the task ID currently highlighted, or "" if none.
func (m BoardModel) SelectedTaskID() string {
	cols := m.board.Columns
	if len(cols) == 0 || m.colIndex >= len(cols) {
		return ""
	}
	colID := cols[m.colIndex].ID
	taskIDs := m.board.Tasks[colID]
	if m.taskIndex >= len(taskIDs) {
		return ""
	}
	return taskIDs[m.taskIndex]
}

// ColIndex returns the currently selected column index.
func (m BoardModel) ColIndex() int { return m.colIndex }

// Board returns the current board state (read-only access for root model).
func (m BoardModel) Board() boarddomain.Board { return m.board }

// View implements tea.Model.
func (m BoardModel) View() string {
	if !m.ready {
		return lipgloss.NewStyle().
			Foreground(m.theme.TextMuted).
			Render("Loading board…")
	}
	if m.width < minBoardWidth || m.height < minBoardHeight {
		return m.singleColumnView()
	}
	return m.fullBoardView()
}

func (m BoardModel) fullBoardView() string {
	cols := m.board.Columns
	if len(cols) == 0 {
		return lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("No columns. Add one with `termino board add-column`.")
	}

	colWidth := (m.width - 2) / len(cols)
	if colWidth < columnMinWidth {
		colWidth = columnMinWidth
	}

	rendered := make([]string, 0, len(cols))
	for i, col := range cols {
		rendered = append(rendered, m.renderColumn(col, i, colWidth))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m BoardModel) singleColumnView() string {
	cols := m.board.Columns
	if len(cols) == 0 || m.colIndex >= len(cols) {
		return lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("No columns.")
	}
	col := cols[m.colIndex]
	return m.renderColumn(col, m.colIndex, m.width-2)
}

func (m BoardModel) renderColumn(col boarddomain.Column, colIdx int, width int) string {
	selected := colIdx == m.colIndex

	headerFg := m.theme.TextMuted
	borderFg := m.theme.Border
	if selected {
		headerFg = m.theme.Accent
		borderFg = m.theme.Accent
	}

	// WIP limit status.
	wipLabel := ""
	taskIDs := m.board.Tasks[col.ID]
	if col.WIPLimit != nil {
		count := len(taskIDs)
		if count >= *col.WIPLimit {
			wipLabel = lipgloss.NewStyle().
				Foreground(m.theme.Danger).
				Render(fmt.Sprintf(" [%d/%d ⚠]", count, *col.WIPLimit))
		} else {
			wipLabel = lipgloss.NewStyle().
				Foreground(m.theme.TextMuted).
				Render(fmt.Sprintf(" [%d/%d]", count, *col.WIPLimit))
		}
	}

	header := lipgloss.NewStyle().
		Foreground(headerFg).
		Bold(selected).
		Width(width - 2).
		Render(col.Name + wipLabel)

	taskLines := make([]string, 0, len(taskIDs))
	for taskIdx, taskID := range taskIDs {
		taskLines = append(taskLines, m.renderTaskLine(taskID, colIdx, taskIdx, width-4))
	}

	body := strings.Join(taskLines, "\n")
	if body == "" {
		body = lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("  (empty)")
	}

	colStyle := lipgloss.NewStyle().
		Border(m.theme.BorderStyle).
		BorderForeground(borderFg).
		Width(width - 2).
		Padding(0, 1)

	return colStyle.Render(header + "\n" + body)
}

func (m BoardModel) renderTaskLine(taskID string, colIdx, taskIdx, width int) string {
	selected := colIdx == m.colIndex && taskIdx == m.taskIndex

	label := taskID
	colID := ""
	cols := m.board.Columns
	if colIdx < len(cols) {
		colID = cols[colIdx].ID
	}
	if colID != "" {
		if fullTasks, ok := m.tasks[colID]; ok && taskIdx < len(fullTasks) {
			t := fullTasks[taskIdx]
			label = fmt.Sprintf("%-8s %s", fmt.Sprintf("#%d", t.Number), t.Title)
		}
	}

	if len(label) > width {
		label = label[:width-1] + "…"
	}

	style := lipgloss.NewStyle().Width(width)
	if selected {
		style = style.
			Background(m.theme.Accent).
			Foreground(m.theme.Surface)
	} else {
		style = style.Foreground(m.theme.Text)
	}
	return style.Render(label)
}

// checkDataVersion queries PRAGMA data_version on the DB and compares with last.
func checkDataVersion(db *sql.DB, lastVersion int) tea.Cmd {
	return func() tea.Msg {
		if db == nil {
			return dataVersionMsg{version: lastVersion}
		}
		var version int
		if err := db.QueryRow("PRAGMA data_version").Scan(&version); err != nil {
			return dataVersionMsg{version: lastVersion, err: err}
		}
		return dataVersionMsg{version: version}
	}
}
