// Package bench holds latency benchmarks for the TRD 14 performance targets,
// run against a temporary SQLite database seeded with realistic data:
//
//   - widget: full summary (open DB + query + render) < 100 ms with 500 tasks
//   - board key response < 50 ms with 1,000 tasks (key + redraw, and the
//     reload that follows a move)
//   - MCP tool calls against the same 1,000-task board
//
// Run with `make bench` or `go test -run '^$' -bench . ./internal/bench`.
package bench

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	appmcp "github.com/lesquel/termino/internal/app/mcp"
	sharedomain "github.com/lesquel/termino/internal/domain"
	agentapp "github.com/lesquel/termino/internal/features/agent/application"
	agentinfra "github.com/lesquel/termino/internal/features/agent/infrastructure"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	widgetapp "github.com/lesquel/termino/internal/features/widget/application"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
	widgetinfra "github.com/lesquel/termino/internal/features/widget/infrastructure"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
	"github.com/lesquel/termino/internal/tuikit"
)

// env is a seeded database plus the use cases wired on top of it.
type env struct {
	path    string
	db      *sql.DB
	project projectdomain.Project
	board   *boardapp.BoardUseCases
	deps    appmcp.Deps
	member  memberdomain.Member
	columns []boarddomain.Column
	taskIDs []string
}

var priorities = []taskdomain.Priority{
	taskdomain.PriorityLow, taskdomain.PriorityMedium, taskdomain.PriorityHigh, taskdomain.PriorityCritical,
}

// seed creates a Kanban project with n tasks spread over its columns; every
// third task is assigned to the member "ana".
func seed(tb testing.TB, n int) env {
	tb.Helper()
	ctx := context.Background()
	path := filepath.Join(tb.TempDir(), "termino.db")
	db, err := sqlite.Open(path)
	if err != nil {
		tb.Fatalf("open db: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })

	bus := events.NewBus()
	boardRepo := boardinfra.NewBoardRepository(db)
	taskRepo := taskinfra.NewTaskRepository(db)
	members := memberapp.NewMemberUseCases(memberinfra.NewMemberRepository(db))
	e := env{
		path:  path,
		db:    db,
		board: boardapp.NewBoardUseCases(boardRepo, taskRepo),
	}
	e.deps = appmcp.Deps{
		ProjectUC: projectapp.NewProjectUseCases(projectinfra.NewProjectRepository(db), boardRepo, bus),
		BoardUC:   e.board,
		TaskUC: taskapp.NewTaskUseCases(taskRepo, taskinfra.NewCommentRepository(db),
			taskinfra.NewActivityRepository(db), boardRepo, bus),
		AgentUC:       agentapp.NewAgentUseCases(agentinfra.NewAgentSessionRepository(db)),
		MemberUC:      members,
		ActorOverride: "bench-agent",
	}

	human := sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
	e.project, err = e.deps.ProjectUC.CreateProject(ctx, human, projectapp.CreateProjectInput{
		Key: "WEB", Name: "Web app", Methodology: projectdomain.MethodologyKanban,
	})
	if err != nil {
		tb.Fatalf("create project: %v", err)
	}
	e.member, err = members.EnsureMember(ctx, "ana", memberdomain.KindHuman)
	if err != nil {
		tb.Fatalf("ensure member: %v", err)
	}
	e.columns, err = boardRepo.ListColumns(ctx, e.project.ID)
	if err != nil || len(e.columns) == 0 {
		tb.Fatalf("list columns: %v (%d)", err, len(e.columns))
	}

	for i := 0; i < n; i++ {
		in := taskapp.CreateTaskInput{
			ProjectID:   e.project.ID,
			Title:       fmt.Sprintf("Task %d: refine the login flow for edge case %d", i+1, i%37),
			Description: "Acceptance: the redirect keeps the original URL and the session survives a reload.",
			ColumnID:    e.columns[i%len(e.columns)].ID,
			Priority:    priorities[i%len(priorities)],
		}
		if i%3 == 0 {
			id := e.member.ID
			in.AssigneeID = &id
		}
		t, err := e.deps.TaskUC.CreateTask(ctx, human, in)
		if err != nil {
			tb.Fatalf("create task %d: %v", i, err)
		}
		e.taskIDs = append(e.taskIDs, t.ID)
	}
	return e
}

// BenchmarkWidgetSummary measures what `termino widget` does after process
// start: open the database (migrations included), resolve the summary and
// render it. TRD 14 target: < 100 ms with 500 tasks.
func BenchmarkWidgetSummary(b *testing.B) {
	e := seed(b, 500)
	ctx := context.Background()
	renderer := widgetinfra.NewNormalRenderer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := widgetOnce(ctx, e, renderer); err != nil {
			b.Fatal(err)
		}
	}
}

// widgetOnce runs one full widget summary: open, query, render, close.
func widgetOnce(ctx context.Context, e env, renderer widgetdomain.Renderer) error {
	db, err := sqlite.Open(e.path)
	if err != nil {
		return err
	}
	defer db.Close()
	uc := widgetapp.NewWidgetUseCases(widgetinfra.NewWidgetRepository(db), nil)
	summary, err := uc.GetSummary(ctx, e.project.ID, e.member.ID, "")
	if err != nil {
		return err
	}
	_, err = renderer.Render(summary, 100)
	return err
}

// BenchmarkBoardLoad measures the reload the board runs after every change
// (e.g. a move key): columns plus all tasks of a 1,000-task project.
func BenchmarkBoardLoad(b *testing.B) {
	e := seed(b, 1000)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := e.board.GetBoardWithTasks(ctx, e.project.ID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBoardKeyResponse measures one navigation key plus the redraw on a
// loaded 1,000-task board. TRD 14 target: < 50 ms.
func BenchmarkBoardKeyResponse(b *testing.B) {
	e := seed(b, 1000)
	m := loadedBoard(b, e)
	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("j")},
		{Type: tea.KeyRunes, Runes: []rune("l")},
		{Type: tea.KeyRunes, Runes: []rune("k")},
		{Type: tea.KeyRunes, Runes: []rune("h")},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(keys[i%len(keys)])
		m = next.(boardtui.BoardModel)
		_ = m.View()
	}
}

func loadedBoard(tb testing.TB, e env) boardtui.BoardModel {
	tb.Helper()
	theme, err := tuikit.Load("default", "")
	if err != nil {
		tb.Fatalf("load theme: %v", err)
	}
	m := boardtui.NewBoardModel(e.project.ID, e.board, nil, theme, tuikit.DefaultKeyMap())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m = next.(boardtui.BoardModel)
	msg := m.Reload()()
	next, _ = m.Update(msg)
	return next.(boardtui.BoardModel)
}

// BenchmarkMCPToolCall measures in-process MCP round trips on a 1,000-task
// board: a read (list_tasks for one column) and a write (move_task).
func BenchmarkMCPToolCall(b *testing.B) {
	e := seed(b, 1000)
	c := mcpClient(b, e)
	b.Run("list_tasks", func(b *testing.B) {
		args := map[string]any{"project_id": e.project.ID, "column_id": e.columns[0].ID}
		for i := 0; i < b.N; i++ {
			call(b, c, "list_tasks", args)
		}
	})
	b.Run("get_board", func(b *testing.B) {
		args := map[string]any{"project_id": e.project.ID}
		for i := 0; i < b.N; i++ {
			call(b, c, "get_board", args)
		}
	})
	b.Run("move_task", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			call(b, c, "move_task", map[string]any{
				"task_id":          e.taskIDs[i%len(e.taskIDs)],
				"target_column_id": e.columns[(i+1)%len(e.columns)].ID,
			})
		}
	})
}

func mcpClient(tb testing.TB, e env) *client.Client {
	tb.Helper()
	c, err := client.NewInProcessClient(appmcp.NewServer(e.deps))
	if err != nil {
		tb.Fatalf("new client: %v", err)
	}
	tb.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		tb.Fatalf("start: %v", err)
	}
	init := mcplib.InitializeRequest{}
	init.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcplib.Implementation{Name: "bench", Version: "1.0"}
	if _, err := c.Initialize(ctx, init); err != nil {
		tb.Fatalf("initialize: %v", err)
	}
	return c
}

func call(tb testing.TB, c *client.Client, name string, args map[string]any) {
	tb.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		tb.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		tb.Fatalf("%s returned a tool error: %v", name, res.Content)
	}
}
