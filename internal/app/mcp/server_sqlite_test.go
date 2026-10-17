package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	appmcp "github.com/lesquel/termino/internal/app/mcp"
	sharedomain "github.com/lesquel/termino/internal/domain"
	agentapp "github.com/lesquel/termino/internal/features/agent/application"
	agentinfra "github.com/lesquel/termino/internal/features/agent/infrastructure"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

// sqliteEnv wires the MCP server against a real SQLite database (foreign
// keys ON, migrations applied) so FK violations surface as they would in
// production.
type sqliteEnv struct {
	deps    appmcp.Deps
	members *memberinfra.MemberRepository
	project projectdomain.Project
}

func newSQLiteEnv(t *testing.T, actorOverride string) sqliteEnv {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "termino.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d (err %v), want 1", fk, err)
	}

	bus := events.NewBus()
	boardRepo := boardinfra.NewBoardRepository(db)
	taskRepo := taskinfra.NewTaskRepository(db)
	members := memberinfra.NewMemberRepository(db)
	deps := appmcp.Deps{
		ProjectUC:     projectapp.NewProjectUseCases(projectinfra.NewProjectRepository(db), boardRepo, bus),
		BoardUC:       boardapp.NewBoardUseCases(boardRepo, taskRepo),
		TaskUC:        taskapp.NewTaskUseCases(taskRepo, taskinfra.NewCommentRepository(db), taskinfra.NewActivityRepository(db), boardRepo, bus),
		AgentUC:       agentapp.NewAgentUseCases(agentinfra.NewAgentSessionRepository(db)),
		MemberUC:      memberapp.NewMemberUseCases(members),
		ActorOverride: actorOverride,
	}
	human := sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
	proj, err := deps.ProjectUC.CreateProject(context.Background(), human, projectapp.CreateProjectInput{
		Key: "WEB", Name: "Web", Methodology: projectdomain.MethodologyKanban,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return sqliteEnv{deps: deps, members: members, project: proj}
}

// connect starts an in-process MCP client that identifies itself as clientName.
func connect(t *testing.T, deps appmcp.Deps, clientName string) *client.Client {
	t.Helper()
	c, err := client.NewInProcessClient(appmcp.NewServer(deps))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	init := mcplib.InitializeRequest{}
	init.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcplib.Implementation{Name: clientName, Version: "1.0"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

// callTool invokes a tool and returns its decoded JSON object, failing the
// test when the tool reports an error.
func callTool(t *testing.T, c *client.Client, name string, args map[string]any) map[string]any {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := ""
	if len(res.Content) > 0 {
		if tc, ok := mcplib.AsTextContent(res.Content[0]); ok {
			text = tc.Text
		}
	}
	if res.IsError {
		t.Fatalf("%s returned tool error: %s", name, text)
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s: decode %q: %v", name, text, err)
	}
	return out
}

func TestCreateTask_SQLite_UsesClientNameAndRegistersAgentMember(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")

	task := callTool(t, c, "create_task", map[string]any{
		"project_id": env.project.ID,
		"title":      "Write docs",
	})

	m, err := env.members.FindByID(context.Background(), "kiro")
	if err != nil {
		t.Fatalf("agent member not registered: %v", err)
	}
	if m.Kind != memberdomain.KindAgent {
		t.Fatalf("member kind = %q, want agent", m.Kind)
	}
	activity, err := env.deps.TaskUC.ListActivityByTask(context.Background(), task["ID"].(string))
	if err != nil || len(activity) == 0 {
		t.Fatalf("activity = %v (err %v), want at least one entry", activity, err)
	}
	if activity[0].ActorID != "kiro" {
		t.Fatalf("activity actor = %q, want kiro", activity[0].ActorID)
	}
}

func TestClaimAndComment_SQLite_ActorOverrideWins(t *testing.T) {
	env := newSQLiteEnv(t, "kiro-ana")
	c := connect(t, env.deps, "kiro")

	task := callTool(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "Fix bug"})
	taskID := task["ID"].(string)
	callTool(t, c, "claim_task", map[string]any{"task_id": taskID, "project_id": env.project.ID})
	callTool(t, c, "add_comment", map[string]any{"task_id": taskID, "body": "on it"})

	got, err := env.deps.TaskUC.FindTaskByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if got.AssigneeID == nil || *got.AssigneeID != "kiro-ana" {
		t.Fatalf("assignee = %v, want kiro-ana", got.AssigneeID)
	}
	if _, err := env.members.FindByID(context.Background(), "kiro"); err == nil {
		t.Fatal("client name registered as member although --actor overrides it")
	}
}

// callToolErr invokes a tool expected to fail and returns its error text.
func callToolErr(t *testing.T, c *client.Client, name string, args map[string]any) string {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s succeeded, want a tool error", name)
	}
	if tc, ok := mcplib.AsTextContent(res.Content[0]); ok {
		return tc.Text
	}
	return ""
}

func TestWriteTools_SQLite_StripTerminalInjection(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")
	ctx := context.Background()

	task := callTool(t, c, "create_task", map[string]any{
		"project_id":  env.project.ID,
		"title":       "\x1b]0;x\x07Fix\nlogin",
		"description": "line1\n\x1b[2Jline2\x07",
	})
	taskID := task["ID"].(string)
	got, err := env.deps.TaskUC.FindTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if got.Title != "Fix login" {
		t.Errorf("title = %q, want %q", got.Title, "Fix login")
	}
	if got.Description != "line1\nline2" {
		t.Errorf("description = %q, want %q", got.Description, "line1\nline2")
	}

	callTool(t, c, "update_task", map[string]any{"task_id": taskID, "title": "\x1b]0;y\x07Renamed"})
	callTool(t, c, "add_comment", map[string]any{"task_id": taskID, "body": "ok\x1b]0;z\x07"})
	got, _ = env.deps.TaskUC.FindTaskByID(ctx, taskID)
	if got.Title != "Renamed" {
		t.Errorf("updated title = %q, want %q", got.Title, "Renamed")
	}
	comments, _ := env.deps.TaskUC.ListCommentsByTask(ctx, taskID)
	if len(comments) != 1 || comments[0].Body != "ok" {
		t.Errorf("comments = %+v, want one body %q", comments, "ok")
	}
}

func TestWriteTools_SQLite_RejectInvalidInput(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")

	msg := callToolErr(t, c, "create_task", map[string]any{
		"project_id": env.project.ID, "title": "Task", "priority": "banana",
	})
	if !strings.Contains(msg, "priority") {
		t.Errorf("error = %q, want it to mention priority", msg)
	}
	callToolErr(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "\x1b]0;x\x07 "})

	task := callTool(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "Task"})
	callToolErr(t, c, "update_task", map[string]any{"task_id": task["ID"], "priority": "urgent"})
}
