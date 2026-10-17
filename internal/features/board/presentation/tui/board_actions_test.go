package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	sharedomain "github.com/lesquel/termino/internal/domain"
	agentdomain "github.com/lesquel/termino/internal/features/agent/domain"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/tuikit"
)

// fakeTaskActions records every mutating call made by the board.
type fakeTaskActions struct {
	moves   []string // "taskID->columnID"
	assigns []string // "taskID->assignee" ("" for unassign)
	deletes []string
	moveErr error
}

func (f *fakeTaskActions) MoveTask(_ context.Context, _ sharedomain.Actor, taskID, col string, _ float64) error {
	if f.moveErr != nil {
		return f.moveErr
	}
	f.moves = append(f.moves, taskID+"->"+col)
	return nil
}

func (f *fakeTaskActions) AssignTask(_ context.Context, _ sharedomain.Actor, taskID string, assignee *string) error {
	a := ""
	if assignee != nil {
		a = *assignee
	}
	f.assigns = append(f.assigns, taskID+"->"+a)
	return nil
}

func (f *fakeTaskActions) DeleteTask(_ context.Context, _ sharedomain.Actor, taskID string, confirm bool) error {
	if confirm {
		f.deletes = append(f.deletes, taskID)
	}
	return nil
}

type fakeMembers struct{ members []memberdomain.Member }

func (f fakeMembers) ListMembers(context.Context) ([]memberdomain.Member, error) {
	return f.members, nil
}

type fakeAgents struct{ sessions []agentdomain.AgentSession }

func (f fakeAgents) ListActiveAgents(context.Context, time.Duration) ([]agentdomain.AgentSession, error) {
	return f.sessions, nil
}

func strp(s string) *string { return &s }

// fixture builds a three-column board whose tasks live in the stub repo, so a
// reload through the board use case returns the same data.
func fixture(t *testing.T, tasks []taskdomain.Task, limit *int) (*stubTaskRepo, *boardapp.BoardUseCases) {
	t.Helper()
	cols := []boarddomain.Column{
		{ID: "col-1", ProjectID: "proj-1", Name: "Backlog", Position: 0},
		{ID: "col-2", ProjectID: "proj-1", Name: "In Progress", Position: 1, WIPLimit: limit},
		{ID: "col-3", ProjectID: "proj-1", Name: "Done", Position: 2},
	}
	repo := &stubTaskRepo{tasks: tasks}
	return repo, boardapp.NewBoardUseCases(&stubBoardRepo{cols: cols}, repo)
}

// run executes cmd and feeds every produced message back into m, expanding
// batches. It skips nil commands and stops after a bounded number of steps.
func run(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0 && steps < 50; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		var next tea.Cmd
		m, next = m.Update(msg)
		queue = append(queue, next)
	}
	return m
}

func press(t *testing.T, m tea.Model, keys ...string) tea.Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		m = run(t, m, cmd)
	}
	return m
}

func newLoadedModel(t *testing.T, uc *boardapp.BoardUseCases, svc boardtui.Services, width int) tea.Model {
	t.Helper()
	m := boardtui.NewBoardModel("proj-1", uc, nil, buildTestTheme(), tuikit.DefaultKeyMap()).WithServices(svc)
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return run(t, model, m.Reload())
}

func baseTasks() []taskdomain.Task {
	return []taskdomain.Task{
		{ID: "t1", ProjectID: "proj-1", Number: 1, Title: "Fix \x1b[31mlogin\x1b]0;pwn\a", ColumnID: "col-1", Priority: taskdomain.PriorityHigh, AssigneeID: strp("bob"), Labels: []string{"bug"}, Position: 1},
		{ID: "t2", ProjectID: "proj-1", Number: 2, Title: "Write docs", ColumnID: "col-1", Priority: taskdomain.PriorityLow, Position: 2},
	}
}

func TestBoardCardsRenderNumberAndSanitizedTitle(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	view := newLoadedModel(t, uc, boardtui.Services{}, 120).View()
	if !strings.Contains(view, "#1 Fix login") {
		t.Errorf("expected card '#1 Fix login' in view:\n%s", view)
	}
	if !strings.Contains(view, "#2 Write docs") {
		t.Errorf("expected card '#2 Write docs' in view:\n%s", view)
	}
	if strings.Contains(view, "pwn") || strings.Contains(view, "t1") {
		t.Errorf("view leaks escape payload or task IDs:\n%s", view)
	}
}

func TestBoardShiftLMovesTaskRight(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	actions := &fakeTaskActions{}
	m := newLoadedModel(t, uc, boardtui.Services{Tasks: actions}, 120)
	press(t, m, "L")
	if len(actions.moves) != 1 || actions.moves[0] != "t1->col-2" {
		t.Errorf("moves = %v, want [t1->col-2]", actions.moves)
	}
}

func TestBoardMovePickerMovesToChosenColumn(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	actions := &fakeTaskActions{}
	m := newLoadedModel(t, uc, boardtui.Services{Tasks: actions}, 120)
	m = press(t, m, "m")
	if !strings.Contains(m.View(), "Move #1") {
		t.Fatalf("expected move picker in view:\n%s", m.View())
	}
	press(t, m, "j", "j", "enter")
	if len(actions.moves) != 1 || actions.moves[0] != "t1->col-3" {
		t.Errorf("moves = %v, want [t1->col-3]", actions.moves)
	}
}

func TestBoardMoveErrorIsShownAsText(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	actions := &fakeTaskActions{moveErr: boardapp.ErrWIPLimitExceeded}
	m := press(t, newLoadedModel(t, uc, boardtui.Services{Tasks: actions}, 120), "L")
	if !strings.Contains(m.View(), "WIP limit exceeded") {
		t.Errorf("expected move error in view:\n%s", m.View())
	}
}

func TestBoardAssignPicksMember(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	actions := &fakeTaskActions{}
	members := fakeMembers{members: []memberdomain.Member{
		{ID: "m-ana", Name: "ana", Kind: memberdomain.KindHuman},
		{ID: "kiro", Name: "kiro", Kind: memberdomain.KindAgent},
	}}
	m := newLoadedModel(t, uc, boardtui.Services{Tasks: actions, Members: members}, 120)
	m = press(t, m, "a")
	if !strings.Contains(m.View(), "Assign #1") || !strings.Contains(m.View(), "kiro") {
		t.Fatalf("expected assign picker listing members:\n%s", m.View())
	}
	// Options: Unassigned, ana, kiro → two downs selects kiro.
	press(t, m, "j", "j", "enter")
	if len(actions.assigns) != 1 || actions.assigns[0] != "t1->kiro" {
		t.Errorf("assigns = %v, want [t1->kiro]", actions.assigns)
	}
}

func TestBoardDeleteRequiresConfirmation(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	actions := &fakeTaskActions{}
	m := newLoadedModel(t, uc, boardtui.Services{Tasks: actions}, 120)
	m = press(t, m, "d")
	if !strings.Contains(m.View(), "Delete #1") {
		t.Fatalf("expected delete confirmation:\n%s", m.View())
	}
	m = press(t, m, "n")
	if len(actions.deletes) != 0 {
		t.Fatalf("delete ran without confirmation: %v", actions.deletes)
	}
	press(t, m, "d", "y")
	if len(actions.deletes) != 1 || actions.deletes[0] != "t1" {
		t.Errorf("deletes = %v, want [t1]", actions.deletes)
	}
}

func TestBoardFilters(t *testing.T) {
	tasks := baseTasks()
	sessions := []agentdomain.AgentSession{{AgentID: "kiro", AgentName: "kiro", CurrentTaskID: strp("t2")}}
	tests := []struct {
		query     string
		want      string
		notWanted string
	}{
		{query: "docs", want: "#2 Write docs", notWanted: "#1 Fix login"},
		{query: "assignee:bob", want: "#1 Fix login", notWanted: "#2 Write docs"},
		{query: "label:bug", want: "#1 Fix login", notWanted: "#2 Write docs"},
		{query: "priority:low", want: "#2 Write docs", notWanted: "#1 Fix login"},
		{query: "agent:kiro", want: "#2 Write docs", notWanted: "#1 Fix login"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			_, uc := fixture(t, tasks, nil)
			m := newLoadedModel(t, uc, boardtui.Services{Agents: fakeAgents{sessions: sessions}}, 120)
			m = press(t, m, "/")
			m = press(t, m, strings.Split(tt.query, "")...)
			m = press(t, m, "enter")
			view := m.View()
			if !strings.Contains(view, tt.want) || strings.Contains(view, tt.notWanted) {
				t.Errorf("filter %q: want %q and not %q in view:\n%s", tt.query, tt.want, tt.notWanted, view)
			}
			if !strings.Contains(view, "Filter: "+tt.query) {
				t.Errorf("active filter not shown as text:\n%s", view)
			}
			// esc clears the filter.
			m = press(t, m, "esc")
			if !strings.Contains(m.View(), tt.notWanted) {
				t.Errorf("esc did not clear filter:\n%s", m.View())
			}
		})
	}
}

func TestBoardFilterCapturesGlobalKeys(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := newLoadedModel(t, uc, boardtui.Services{}, 120)
	m = press(t, m, "/")
	if !m.(boardtui.BoardModel).Capturing() {
		t.Error("board must capture keys while the filter prompt is open")
	}
}

func TestBoardWorkingMarker(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	sessions := []agentdomain.AgentSession{{AgentID: "kiro", AgentName: "kiro", CurrentTaskID: strp("t2")}}
	view := newLoadedModel(t, uc, boardtui.Services{Agents: fakeAgents{sessions: sessions}}, 120).View()
	if !strings.Contains(view, "kiro working") {
		t.Errorf("expected 'kiro working' marker:\n%s", view)
	}
}

func TestBoardWIPExceededIsMarkedWithText(t *testing.T) {
	limit := 1
	tasks := []taskdomain.Task{
		{ID: "a", ProjectID: "proj-1", Number: 1, Title: "A", ColumnID: "col-2", Position: 1},
		{ID: "b", ProjectID: "proj-1", Number: 2, Title: "B", ColumnID: "col-2", Position: 2},
	}
	_, uc := fixture(t, tasks, &limit)
	view := newLoadedModel(t, uc, boardtui.Services{}, 120).View()
	if !strings.Contains(view, "2/1 WIP exceeded") {
		t.Errorf("expected WIP exceeded text:\n%s", view)
	}
}

func TestBoardNarrowViewShowsOneColumn(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := newLoadedModel(t, uc, boardtui.Services{}, 60)
	view := m.View()
	if !strings.Contains(view, "Backlog") || strings.Contains(view, "In Progress") {
		t.Fatalf("narrow view should only show Backlog:\n%s", view)
	}
	if !strings.Contains(view, "1/3") {
		t.Errorf("narrow view should show column position 1/3:\n%s", view)
	}
	view = press(t, m, "l").View()
	if !strings.Contains(view, "In Progress") || !strings.Contains(view, "2/3") || strings.Contains(view, "Backlog") {
		t.Errorf("after l the narrow view should show In Progress 2/3:\n%s", view)
	}
}

func TestBoardMoveToDoneCelebratesWithMotion(t *testing.T) {
	tasks := []taskdomain.Task{{ID: "t1", ProjectID: "proj-1", Number: 1, Title: "Ship", ColumnID: "col-2", Position: 1}}
	_, uc := fixture(t, tasks, nil)
	var done []string
	svc := boardtui.Services{Tasks: &fakeTaskActions{}, Motion: true, OnDone: func(id string) { done = append(done, id) }}
	m := newLoadedModel(t, uc, svc, 120)
	m = press(t, m, "l") // select In Progress

	// Press L and inspect the model before celebration ticks run.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	m, cmd = m.Update(cmd())
	bm := m.(boardtui.BoardModel)
	if !bm.Celebrating() {
		t.Fatal("expected celebration after moving to Done with motion enabled")
	}
	if !strings.Contains(bm.View(), "#1 done") {
		t.Errorf("expected celebration text:\n%s", bm.View())
	}
	if len(done) != 1 || done[0] != "t1" {
		t.Errorf("OnDone calls = %v, want [t1]", done)
	}
	m = run(t, m, cmd)
	if m.(boardtui.BoardModel).Celebrating() {
		t.Error("celebration must end after its frames")
	}
}

func TestBoardMoveToDoneWithoutMotionIsStatic(t *testing.T) {
	tasks := []taskdomain.Task{{ID: "t1", ProjectID: "proj-1", Number: 1, Title: "Ship", ColumnID: "col-2", Position: 1}}
	_, uc := fixture(t, tasks, nil)
	m := newLoadedModel(t, uc, boardtui.Services{Tasks: &fakeTaskActions{}, Motion: false}, 120)
	m = press(t, m, "l")
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	m, _ = m.Update(cmd())
	bm := m.(boardtui.BoardModel)
	if bm.Celebrating() {
		t.Error("no animation may run when motion is disabled")
	}
	if !strings.Contains(bm.View(), "#1 done") {
		t.Errorf("expected static done text:\n%s", bm.View())
	}
}
