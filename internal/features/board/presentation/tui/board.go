// Package tui provides the TUI presentation layer for the board feature.
package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/lesquel/termino/internal/tuikit"
)

const (
	pollInterval   = 500 * time.Millisecond
	minBoardWidth  = 80
	minBoardHeight = 24
	columnMinWidth = 20
	minHeaderName  = 6 // column-name cells kept before a WIP badge shortens

	// celebrationFrames × celebrationInterval stays under 1 s (TRD 10.5).
	celebrationFrames   = 6
	celebrationInterval = 120 * time.Millisecond

	// statusTTL is how long a status message stays before the poll tick
	// clears it.
	statusTTL = 3 * time.Second
)

// BoardLoadedMsg carries a refreshed board from an async load.
type BoardLoadedMsg struct {
	Board   boarddomain.Board
	Tasks   map[string][]taskdomain.Task // colID → tasks
	Members map[string]string            // member ID → display name
	Working map[string]string            // task ID → name of the agent working on it
	Err     error
}

// RefreshMsg triggers a board reload when the data_version pragma changes.
// Re-exported here as an alias from tuikit.
type RefreshMsg = tuikit.RefreshMsg

// dataVersionMsg carries the current SQLite data_version.
type dataVersionMsg struct {
	version int
	err     error
}

// actionResultMsg reports the outcome of a move/assign/delete.
type actionResultMsg struct {
	verb string // "moved", "assigned", "deleted"
	task taskdomain.Task
	to   string // target column or assignee display name
	toID string // target column ID for moves
	done bool   // the task landed in the Done column
	err  error
}

// membersMsg carries the members listed for the assign picker.
type membersMsg struct {
	options []pickerOption
	err     error
}

// celebrationTickMsg advances the Done celebration animation.
type celebrationTickMsg struct{}

// mode is the board's input mode; anything but modeNormal captures keys.
type mode int

const (
	modeNormal mode = iota
	modeMove
	modeAssign
	modeConfirmDelete
	modeFilter
)

// pickerOption is one entry of the move/assign pickers.
type pickerOption struct {
	label string
	value string // column ID or member ID ("" = unassign)
}

// card is a rendered board entry: a full task when available, otherwise a
// bare task ID (boards loaded without task details).
type card struct {
	id   string
	task *taskdomain.Task
}

// BoardModel is the Bubble Tea model for the Kanban board view.
type BoardModel struct {
	board     boarddomain.Board
	tasks     map[string][]taskdomain.Task // colID → ordered tasks
	members   map[string]string            // member ID → name
	working   map[string]string            // task ID → agent name
	colIndex  int                          // selected column index
	taskIndex int                          // selected card index within the visible column
	theme     *tuikit.Theme
	keys      tuikit.KeyMap
	boardUC   *boardapp.BoardUseCases
	svc       Services
	db        *sql.DB // for live polling (read-only connection)
	projectID string
	width     int
	height    int
	ready     bool
	dataVer   int

	mode        mode
	target      taskdomain.Task // task the open picker/confirmation acts on
	picker      []pickerOption
	pickerIdx   int
	filterInput textinput.Model
	filter      Filter
	filterQuery string

	status      string
	statusErr   bool
	statusAt    time.Time
	now         func() time.Time
	celebration int // remaining animation frames
	celebrated  string

	focusTaskID string // card to select once the next reload lands

	projectName string
	projectKey  string
}

// NewBoardModel creates a new BoardModel for the given project.
func NewBoardModel(
	projectID string,
	boardUC *boardapp.BoardUseCases,
	db *sql.DB,
	theme *tuikit.Theme,
	keys tuikit.KeyMap,
) BoardModel {
	ti := textinput.New()
	ti.Placeholder = "text assignee:name label:x priority:high agent:name"
	ti.Prompt = "/ "
	ti.CharLimit = 200
	return BoardModel{
		projectID:   projectID,
		boardUC:     boardUC,
		db:          db,
		theme:       theme,
		keys:        keys,
		tasks:       make(map[string][]taskdomain.Task),
		filterInput: ti,
		now:         time.Now,
	}
}

// WithProject returns a copy of the model that shows the project's name in
// its header and refers to tasks as KEY-N.
func (m BoardModel) WithProject(name, key string) BoardModel {
	m.projectName = name
	m.projectKey = key
	return m
}

// WithClock returns a copy of the model that reads time from now (tests).
func (m BoardModel) WithClock(now func() time.Time) BoardModel {
	m.now = now
	return m
}

// TaskRef returns the display reference of task number n: KEY-N when the
// project key is known, otherwise #N.
func (m BoardModel) TaskRef(n int) string {
	if m.projectKey != "" {
		return fmt.Sprintf("%s-%d", m.projectKey, n)
	}
	return fmt.Sprintf("#%d", n)
}

// MemberNames returns the member ID → name map of the last load.
func (m BoardModel) MemberNames() map[string]string { return m.members }

// Created confirms a task created outside the board (the new-task form) and
// selects it once the next reload lands.
func (m BoardModel) Created(t taskdomain.Task) (BoardModel, tea.Cmd) {
	m.setStatus("Created "+m.TaskRef(t.Number)+" "+sanitize.Line(t.Title), false)
	m.focusTaskID = t.ID
	return m, m.Reload()
}

// WithServices returns a copy of the model wired to the given collaborators.
func (m BoardModel) WithServices(svc Services) BoardModel {
	m.svc = svc
	return m
}

// Init implements tea.Model — starts loading the board and the polling tick.
func (m BoardModel) Init() tea.Cmd {
	return tea.Batch(m.Reload(), tickPoll())
}

// Reload returns a command that reloads the board, its tasks, member names
// and agent presence.
func (m BoardModel) Reload() tea.Cmd {
	return loadBoard(m.boardUC, m.svc, m.projectID)
}

func loadBoard(uc *boardapp.BoardUseCases, svc Services, projectID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		board, tasks, err := uc.GetBoardWithTasks(ctx, projectID)
		if err != nil {
			return BoardLoadedMsg{Err: err}
		}
		msg := BoardLoadedMsg{Board: board, Tasks: tasks}
		if svc.Members != nil {
			if members, err := svc.Members.ListMembers(ctx); err == nil {
				msg.Members = make(map[string]string, len(members))
				for _, mem := range members {
					msg.Members[mem.ID] = mem.Name
				}
			}
		}
		if svc.Agents != nil {
			if sessions, err := svc.Agents.ListActiveAgents(ctx, activeAgentWindow); err == nil {
				msg.Working = make(map[string]string, len(sessions))
				for _, s := range sessions {
					if s.CurrentTaskID == nil {
						continue
					}
					name := s.AgentName
					if name == "" {
						name = s.AgentID
					}
					msg.Working[*s.CurrentTaskID] = name
				}
			}
		}
		return msg
	}
}

func tickPoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return tuikit.RefreshMsg{}
	})
}

func tickCelebration() tea.Cmd {
	return tea.Tick(celebrationInterval, func(time.Time) tea.Msg {
		return celebrationTickMsg{}
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
			m.setStatus(msg.Err.Error(), true)
			return m, nil
		}
		m.board = msg.Board
		if msg.Tasks != nil {
			m.tasks = msg.Tasks
		}
		m.members = msg.Members
		m.working = msg.Working
		m.ready = true
		m.applyFocus()
		m.clampSelection()

	case tuikit.RefreshMsg:
		if m.status != "" && m.now().Sub(m.statusAt) >= statusTTL {
			m.status, m.statusErr = "", false
		}
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
			return m, m.Reload()
		}

	case actionResultMsg:
		return m.handleActionResult(msg)

	case membersMsg:
		if msg.err != nil {
			m.setStatus("list members: "+msg.err.Error(), true)
			return m, nil
		}
		m.mode = modeAssign
		m.picker = msg.options
		m.pickerIdx = 0

	case celebrationTickMsg:
		if m.celebration > 0 {
			m.celebration--
		}
		if m.celebration > 0 {
			return m, tickCelebration()
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m BoardModel) handleActionResult(msg actionResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("Could not update %s: %v", m.TaskRef(msg.task.Number), msg.err), true)
		return m, nil
	}
	if msg.toID != "" {
		m.followTo(msg.task.ID, msg.toID)
	}
	if !msg.done {
		m.setStatus(fmt.Sprintf("%s %s %s", m.TaskRef(msg.task.Number), msg.verb, sanitize.Line(msg.to)), false)
		return m, m.Reload()
	}
	m.celebrated = m.TaskRef(msg.task.Number) + " done"
	if !m.svc.Motion {
		// Motion disabled (config or NO_COLOR): a static, text-only confirmation.
		m.setStatus(m.doneSymbol()+" "+m.celebrated+": "+sanitize.Line(msg.task.Title), false)
		return m, m.Reload()
	}
	m.status = ""
	m.celebration = celebrationFrames
	return m, tea.Batch(m.Reload(), tickCelebration())
}

func (m *BoardModel) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
	m.statusAt = m.now()
}

// Capturing reports whether the board is consuming keystrokes (a prompt or
// picker is open), so the root model must not treat them as global keys.
func (m BoardModel) Capturing() bool { return m.mode != modeNormal }

// Celebrating reports whether the Done celebration animation is running.
func (m BoardModel) Celebrating() bool { return m.celebration > 0 }

// cards returns the visible cards of a column, honoring the active filter.
func (m BoardModel) cards(colID string) []card {
	if full, ok := m.tasks[colID]; ok {
		out := make([]card, 0, len(full))
		for i := range full {
			t := &full[i]
			if !m.filter.Empty() && !m.filter.Match(*t, m.assigneeName(*t), m.working[t.ID]) {
				continue
			}
			out = append(out, card{id: t.ID, task: t})
		}
		return out
	}
	ids := m.board.Tasks[colID]
	out := make([]card, 0, len(ids))
	for _, id := range ids {
		out = append(out, card{id: id})
	}
	return out
}

func (m BoardModel) assigneeName(t taskdomain.Task) string {
	if t.AssigneeID == nil {
		return ""
	}
	if name, ok := m.members[*t.AssigneeID]; ok {
		return name
	}
	return *t.AssigneeID
}

func (m *BoardModel) clampSelection() {
	cols := m.board.Columns
	if len(cols) == 0 {
		return
	}
	if m.colIndex >= len(cols) {
		m.colIndex = len(cols) - 1
	}
	n := len(m.cards(cols[m.colIndex].ID))
	if m.taskIndex >= n {
		m.taskIndex = max(n-1, 0)
	}
}

// selectFirstMatch moves to the first column with a visible card when the
// selected column has none (e.g. right after applying a filter).
func (m *BoardModel) selectFirstMatch() {
	cols := m.board.Columns
	if m.colIndex < len(cols) && len(m.cards(cols[m.colIndex].ID)) > 0 {
		return
	}
	for i, col := range cols {
		if len(m.cards(col.ID)) > 0 {
			m.colIndex = i
			m.applyFocus()
			return
		}
	}
}

// prioritySymbol is the theme symbol drawn before a card; medium (the
// default priority) has none so ordinary cards stay uncluttered.
func (m BoardModel) prioritySymbol(p taskdomain.Priority) string {
	if p == "" || p == taskdomain.PriorityMedium {
		return ""
	}
	return sanitize.Line(m.theme.Symbols[string(p)])
}

// followTo moves the selection to the column a card was moved into and
// remembers the card so the next reload selects it.
func (m *BoardModel) followTo(taskID, colID string) {
	for i, col := range m.board.Columns {
		if col.ID == colID {
			m.colIndex = i
			m.focusTaskID = taskID
			return
		}
	}
}

// applyFocus selects the pending focus card if it is visible in the
// selected column.
func (m *BoardModel) applyFocus() {
	if m.focusTaskID == "" || m.colIndex >= len(m.board.Columns) {
		return
	}
	for i, c := range m.cards(m.board.Columns[m.colIndex].ID) {
		if c.id == m.focusTaskID {
			m.taskIndex = i
			break
		}
	}
	m.focusTaskID = ""
}

// selectedTask returns the highlighted task when full task data is loaded.
func (m BoardModel) selectedTask() (taskdomain.Task, bool) {
	cols := m.board.Columns
	if len(cols) == 0 || m.colIndex >= len(cols) {
		return taskdomain.Task{}, false
	}
	cs := m.cards(cols[m.colIndex].ID)
	if m.taskIndex >= len(cs) || cs[m.taskIndex].task == nil {
		return taskdomain.Task{}, false
	}
	return *cs[m.taskIndex].task, true
}

func (m BoardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeMove, modeAssign:
		return m.handlePickerKey(msg)
	case modeConfirmDelete:
		m.mode = modeNormal
		if msg.String() == "y" || msg.String() == "Y" {
			return m, m.deleteCmd(m.target)
		}
		m.setStatus("Delete cancelled", false)
		return m, nil
	}

	cols := m.board.Columns
	if len(cols) == 0 {
		return m, nil
	}
	if runes, ok := navigationBurst(msg); ok {
		// Keys that arrived together in one message (fast typing or key
		// repeat): apply each navigation key in order.
		var model tea.Model = m
		for _, r := range runes {
			model, _ = model.(BoardModel).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		return model, nil
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
		if m.taskIndex < len(m.cards(cols[m.colIndex].ID))-1 {
			m.taskIndex++
		}
	case "H", "shift+left":
		return m.moveSelectedBy(-1)
	case "L", "shift+right":
		return m.moveSelectedBy(1)
	case "m":
		return m.openMovePicker()
	case "a":
		return m.openAssignPicker()
	case "d":
		if t, ok := m.selectedTask(); ok && m.svc.Tasks != nil {
			m.target = t
			m.mode = modeConfirmDelete
		}
	case "/":
		m.mode = modeFilter
		m.filterInput.SetValue(m.filterQuery)
		m.filterInput.CursorEnd()
		return m, m.filterInput.Focus()
	case "esc":
		if !m.filter.Empty() {
			m.filter, m.filterQuery = Filter{}, ""
			m.clampSelection()
		}
		m.status = ""
	}
	return m, nil
}

// navigationBurst returns the runes of a multi-rune key message made only of
// the h/j/k/l navigation keys. Moves (H/L) are left out: each move is computed
// from the pre-move selection, so a burst would repeat the same move.
func navigationBurst(msg tea.KeyMsg) ([]rune, bool) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) < 2 || msg.Alt || msg.Paste {
		return nil, false
	}
	for _, r := range msg.Runes {
		if !strings.ContainsRune("hjkl", r) {
			return nil, false
		}
	}
	return msg.Runes, true
}

func (m BoardModel) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.filterQuery = strings.TrimSpace(sanitize.Line(m.filterInput.Value()))
		m.filter = ParseFilter(m.filterQuery)
		m.mode = modeNormal
		m.filterInput.Blur()
		m.taskIndex = 0
		m.selectFirstMatch()
		m.clampSelection()
		return m, nil
	case tea.KeyEsc:
		m.mode = modeNormal
		m.filterInput.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

func (m BoardModel) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k := msg.String(); k {
	case "esc", "q":
		m.mode = modeNormal
		return m, nil
	case "up", "k":
		if m.pickerIdx > 0 {
			m.pickerIdx--
		}
	case "down", "j":
		if m.pickerIdx < len(m.picker)-1 {
			m.pickerIdx++
		}
	case "enter":
		return m.choosePicker(m.pickerIdx)
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			if idx := int(k[0] - '1'); idx < len(m.picker) {
				return m.choosePicker(idx)
			}
		}
	}
	return m, nil
}

func (m BoardModel) choosePicker(idx int) (tea.Model, tea.Cmd) {
	if idx >= len(m.picker) {
		return m, nil
	}
	choice := m.picker[idx]
	current := m.mode
	m.mode = modeNormal
	switch current {
	case modeMove:
		for _, col := range m.board.Columns {
			if col.ID == choice.value {
				return m, m.moveCmd(m.target, col)
			}
		}
	case modeAssign:
		return m, m.assignCmd(m.target, choice)
	}
	return m, nil
}

func (m BoardModel) moveSelectedBy(delta int) (tea.Model, tea.Cmd) {
	t, ok := m.selectedTask()
	target := m.colIndex + delta
	if !ok || m.svc.Tasks == nil || target < 0 || target >= len(m.board.Columns) {
		return m, nil
	}
	return m, m.moveCmd(t, m.board.Columns[target])
}

func (m BoardModel) openMovePicker() (tea.Model, tea.Cmd) {
	t, ok := m.selectedTask()
	if !ok || m.svc.Tasks == nil {
		return m, nil
	}
	m.target = t
	m.picker = make([]pickerOption, 0, len(m.board.Columns))
	for _, col := range m.board.Columns {
		m.picker = append(m.picker, pickerOption{label: sanitize.Line(col.Name), value: col.ID})
	}
	m.pickerIdx = m.colIndex
	m.mode = modeMove
	return m, nil
}

func (m BoardModel) openAssignPicker() (tea.Model, tea.Cmd) {
	t, ok := m.selectedTask()
	if !ok || m.svc.Tasks == nil {
		return m, nil
	}
	if m.svc.Members == nil {
		m.setStatus("No members available to assign", true)
		return m, nil
	}
	m.target = t
	members := m.svc.Members
	return m, func() tea.Msg {
		list, err := members.ListMembers(context.Background())
		if err != nil {
			return membersMsg{err: err}
		}
		opts := []pickerOption{{label: "Unassigned"}}
		for _, mem := range list {
			if mem.System() {
				continue // built-in attribution members never take work
			}
			opts = append(opts, pickerOption{
				label: fmt.Sprintf("%s (%s)", sanitize.Line(mem.Name), mem.Kind),
				value: mem.ID,
			})
		}
		return membersMsg{options: opts}
	}
}

func (m BoardModel) moveCmd(t taskdomain.Task, col boarddomain.Column) tea.Cmd {
	if t.ColumnID == col.ID {
		return nil
	}
	svc := m.svc
	position := 1.0
	if existing := m.tasks[col.ID]; len(existing) > 0 {
		position = existing[len(existing)-1].Position + 1
	}
	doneCol, hasDone := boarddomain.DoneColumn(m.board.Columns)
	isDone := hasDone && doneCol.ID == col.ID
	return func() tea.Msg {
		err := svc.Tasks.MoveTask(context.Background(), svc.Actor, t.ID, col.ID, position)
		if err == nil && isDone && svc.OnDone != nil {
			svc.OnDone(t.ID)
		}
		return actionResultMsg{verb: "moved to", task: t, to: col.Name, toID: col.ID, done: isDone, err: err}
	}
}

func (m BoardModel) assignCmd(t taskdomain.Task, choice pickerOption) tea.Cmd {
	svc := m.svc
	var assignee *string
	to := "nobody"
	if choice.value != "" {
		v := choice.value
		assignee = &v
		to = choice.label
	}
	return func() tea.Msg {
		err := svc.Tasks.AssignTask(context.Background(), svc.Actor, t.ID, assignee)
		return actionResultMsg{verb: "assigned to", task: t, to: to, err: err}
	}
}

func (m BoardModel) deleteCmd(t taskdomain.Task) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		err := svc.Tasks.DeleteTask(context.Background(), svc.Actor, t.ID, true)
		return actionResultMsg{verb: "deleted", task: t, err: err}
	}
}

// SelectedTaskID returns the task ID currently highlighted, or "" if none.
func (m BoardModel) SelectedTaskID() string {
	cols := m.board.Columns
	if len(cols) == 0 || m.colIndex >= len(cols) {
		return ""
	}
	cs := m.cards(cols[m.colIndex].ID)
	if m.taskIndex >= len(cs) {
		return ""
	}
	return cs[m.taskIndex].id
}

// ColIndex returns the currently selected column index.
func (m BoardModel) ColIndex() int { return m.colIndex }

// Board returns the current board state (read-only access for root model).
func (m BoardModel) Board() boarddomain.Board { return m.board }

// View implements tea.Model. The layout is fixed: a header line, one
// status row (blank when idle, so messages never shift the board), the
// columns, and a footer with the available keys or the open prompt.
func (m BoardModel) View() string {
	if !m.ready {
		return lipgloss.NewStyle().
			Foreground(m.theme.TextMuted).
			Render("Loading board…")
	}
	var body string
	if m.width < minBoardWidth || m.height < minBoardHeight {
		body = m.singleColumnView()
	} else {
		body = m.fullBoardView()
	}
	bottom := m.bottomBar()
	if bottom == "" {
		bottom = m.footer()
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.header(), m.statusRow(), body, bottom)
}

// header shows the project and the active filter.
func (m BoardModel) header() string {
	title := "termino"
	if m.projectName != "" {
		title += " · " + sanitize.Line(m.projectName)
		if m.projectKey != "" {
			title += " (" + m.projectKey + ")"
		}
	}
	out := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render(title)
	if m.filterQuery != "" {
		out += lipgloss.NewStyle().Foreground(m.theme.Warning).
			Render("  Filter: " + m.filterQuery + "  (esc clears)")
	}
	return m.fit(out)
}

// statusRow is always one line: the celebration, the last status, an
// empty-state hint, or a blank line.
func (m BoardModel) statusRow() string {
	var line string
	switch {
	case m.celebration > 0:
		sparkles := []string{"✦", "✧", "★", "✧"}
		s := sparkles[m.celebration%len(sparkles)]
		line = lipgloss.NewStyle().Foreground(m.theme.Success).Bold(true).
			Render(fmt.Sprintf("%s %s %s! %s", s, m.doneSymbol(), m.celebrated, s))
	case m.status != "":
		fg := m.theme.TextMuted
		if m.statusErr {
			fg = m.theme.Danger
		}
		line = lipgloss.NewStyle().Foreground(fg).Render(m.status)
	case m.visibleCount() == 0 && len(m.board.Columns) > 0:
		hint := "No tasks yet — press n to create one"
		if !m.filter.Empty() {
			hint = "No matches — press esc to clear the filter"
		}
		line = lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(hint)
	}
	if line == "" {
		return " "
	}
	return m.fit(line)
}

// visibleCount is the number of cards shown across all columns.
func (m BoardModel) visibleCount() int {
	n := 0
	for _, col := range m.board.Columns {
		n += len(m.cards(col.ID))
	}
	return n
}

// fit truncates a rendered line to the board width, when known.
func (m BoardModel) fit(s string) string {
	if m.width <= 0 {
		return s
	}
	return ansi.Truncate(s, m.width, "…")
}

// footerHint is one "key action" pair of the footer.
type footerHint struct{ key, desc string }

// footerHints are listed most useful first; the footer keeps as many as fit
// and always ends with help and quit.
var footerHints = []footerHint{
	{"n", "new"},
	{"enter", "open"},
	{"/", "filter"},
	{"H/L", "move"},
	{"a", "assign"},
	{"m", "move to"},
	{"d", "delete"},
	{"p", "projects"},
	{"hjkl", "navigate"},
}

var footerTail = []footerHint{{"?", "help"}, {"q", "quit"}}

// footer renders the key hints that fit the board width. Text carries the
// meaning, so it reads the same under NO_COLOR.
func (m BoardModel) footer() string {
	const sep = "  "
	width := m.width
	if width <= 0 {
		width = minBoardWidth
	}
	hintWidth := func(h footerHint) int { return ansi.StringWidth(h.key) + 1 + ansi.StringWidth(h.desc) }
	used := 0
	for _, h := range footerTail {
		used += hintWidth(h) + len(sep)
	}
	var chosen []footerHint
	for _, h := range footerHints {
		if used+hintWidth(h)+len(sep) > width {
			break
		}
		chosen = append(chosen, h)
		used += hintWidth(h) + len(sep)
	}
	chosen = append(chosen, footerTail...)

	keyStyle := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(m.theme.TextMuted)
	parts := make([]string, 0, len(chosen))
	for _, h := range chosen {
		parts = append(parts, keyStyle.Render(h.key)+" "+descStyle.Render(h.desc))
	}
	return ansi.Truncate(strings.Join(parts, sep), width, "…")
}

// bottomBar renders the open prompt or picker, or "" in normal mode.
func (m BoardModel) bottomBar() string {
	box := lipgloss.NewStyle().Border(m.theme.BorderStyle).BorderForeground(m.theme.Accent).Padding(0, 1)
	title := m.TaskRef(m.target.Number) + " " + sanitize.Line(m.target.Title)
	switch m.mode {
	case modeFilter:
		return box.Render("Filter cards\n" + m.filterInput.View() + "\nenter=apply  esc=cancel")
	case modeConfirmDelete:
		return box.BorderForeground(m.theme.Danger).
			Render(fmt.Sprintf("Delete %s? (y/N)", title))
	case modeMove, modeAssign:
		head := "Move " + title + " to:"
		if m.mode == modeAssign {
			head = "Assign " + title + " to:"
		}
		lines := []string{head}
		for i, opt := range m.picker {
			cursor := "  "
			if i == m.pickerIdx {
				cursor = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%d. %s", cursor, i+1, opt.label))
		}
		lines = append(lines, "enter=choose  esc=cancel")
		return box.Render(strings.Join(lines, "\n"))
	}
	return ""
}

func (m BoardModel) doneSymbol() string {
	if s, ok := m.theme.Symbols["done"]; ok && s != "" {
		return s
	}
	return "✓"
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

// singleColumnView is the narrow layout (< 80 columns, PRD 5.6): one column
// at a time with a text position indicator; h/l switch columns.
func (m BoardModel) singleColumnView() string {
	cols := m.board.Columns
	if len(cols) == 0 || m.colIndex >= len(cols) {
		return lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render("No columns.")
	}
	left, right := " ", " "
	if m.colIndex > 0 {
		left = "◀"
	}
	if m.colIndex < len(cols)-1 {
		right = "▶"
	}
	nav := lipgloss.NewStyle().Foreground(m.theme.TextMuted).
		Render(fmt.Sprintf("%s column %d/%d %s  (h/l)", left, m.colIndex+1, len(cols), right))
	width := m.width
	if width < columnMinWidth {
		width = columnMinWidth
	}
	return nav + "\n" + m.renderColumn(cols[m.colIndex], m.colIndex, width-2)
}

func (m BoardModel) renderColumn(col boarddomain.Column, colIdx int, width int) string {
	selected := colIdx == m.colIndex

	headerFg := m.theme.TextMuted
	borderFg := m.theme.Border
	if selected {
		headerFg = m.theme.Accent
		borderFg = m.theme.Accent
	}

	// WIP limit status: always text, never color alone (PRD 5.6).
	name := sanitize.Line(col.Name)
	headerWidth := width - 4 // column width minus border and padding
	wipLabel := ""
	if col.WIPLimit != nil {
		count := len(m.board.Tasks[col.ID])
		var style lipgloss.Style
		var badges []string // longest first; the first that fits is used
		switch {
		case boarddomain.WIPExceeded(m.board, col):
			borderFg = m.theme.Danger
			style = lipgloss.NewStyle().Foreground(m.theme.Danger).Bold(true)
			badges = []string{
				fmt.Sprintf(" [⚠ %d/%d WIP exceeded]", count, *col.WIPLimit),
				fmt.Sprintf(" [⚠ %d/%d over]", count, *col.WIPLimit),
				fmt.Sprintf(" [⚠ %d/%d]", count, *col.WIPLimit),
			}
		case count == *col.WIPLimit:
			style = lipgloss.NewStyle().Foreground(m.theme.Warning)
			badges = []string{
				fmt.Sprintf(" [%d/%d full]", count, *col.WIPLimit),
				fmt.Sprintf(" [%d/%d]", count, *col.WIPLimit),
			}
		default:
			style = lipgloss.NewStyle().Foreground(m.theme.TextMuted)
			badges = []string{fmt.Sprintf(" [%d/%d]", count, *col.WIPLimit)}
		}
		badge := fitBadge(badges, ansi.StringWidth(name), headerWidth)
		name = ansi.Truncate(name, max(headerWidth-ansi.StringWidth(badge), 1), "…")
		wipLabel = style.Render(badge)
	} else {
		name = ansi.Truncate(name, max(headerWidth, 1), "…")
	}

	header := lipgloss.NewStyle().
		Foreground(headerFg).
		Bold(selected).
		Width(headerWidth).
		Render(name + wipLabel)

	cs := m.cards(col.ID)
	taskLines := make([]string, 0, len(cs))
	for idx, c := range cs {
		taskLines = append(taskLines, m.renderCard(c, selected && idx == m.taskIndex, width-4))
	}

	body := strings.Join(taskLines, "\n")
	if body == "" {
		empty := "  (empty)"
		if !m.filter.Empty() {
			empty = "  (no matches)"
		}
		body = lipgloss.NewStyle().Foreground(m.theme.TextMuted).Render(empty)
	}

	colStyle := lipgloss.NewStyle().
		Border(m.theme.BorderStyle).
		BorderForeground(borderFg).
		Width(width-2).
		Padding(0, 1)

	return colStyle.Render(header + "\n" + body)
}

// fitBadge picks the longest badge that leaves room for the column name (or
// at least minHeaderName cells of it), so the header never wraps.
func fitBadge(badges []string, nameWidth, width int) string {
	for _, b := range badges {
		if width-ansi.StringWidth(b) >= min(nameWidth, minHeaderName) {
			return b
		}
	}
	return badges[len(badges)-1]
}

// renderCard draws "[priority] #N Title @assignee", plus a "working" line when an active
// agent claimed the task. All task text is sanitized before drawing.
func (m BoardModel) renderCard(c card, selected bool, width int) string {
	width = max(width, 4)
	label := sanitize.Line(c.id)
	working := ""
	if c.task != nil {
		t := *c.task
		label = m.TaskRef(t.Number) + " " + sanitize.Line(t.Title)
		if sym := m.prioritySymbol(t.Priority); sym != "" {
			label = sym + " " + label
		}
		if name := m.assigneeName(t); name != "" {
			label += " @" + sanitize.Line(name)
		}
		if agent := m.working[t.ID]; agent != "" {
			working = "  ⚙ " + sanitize.Line(agent) + " working"
		}
	}
	// The selection marker is text, so it survives NO_COLOR (PRD 5.6).
	marker := "  "
	if selected {
		marker = "> "
	}
	label = marker + ansi.Truncate(label, width-len(marker), "…")

	style := lipgloss.NewStyle().Width(width)
	if selected {
		style = style.Background(m.theme.Accent).Foreground(m.theme.Surface)
	} else {
		style = style.Foreground(m.theme.Text)
	}
	out := style.Render(label)
	if working != "" {
		out += "\n" + lipgloss.NewStyle().Foreground(m.theme.Warning).Width(width).
			Render(ansi.Truncate(working, width, "…"))
	}
	return out
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
