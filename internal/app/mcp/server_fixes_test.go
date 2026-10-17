package mcp_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	appmcp "github.com/lesquel/termino/internal/app/mcp"
	sharedomain "github.com/lesquel/termino/internal/domain"
)

func TestDeleteTaskWithoutConfirm_UsesRealProjectKey(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")
	task := callTool(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "Old"})

	msg := callToolErr(t, c, "delete_task", map[string]any{"task_id": task["ID"].(string)})
	if !strings.Contains(msg, "WEB-1") || strings.Contains(msg, "TER-") {
		t.Errorf("message should name WEB-1, got %q", msg)
	}
}

func TestCompleteTask_UsesColumnNamedDone(t *testing.T) {
	env := newSQLiteEnv(t, "")
	ctx := context.Background()
	// A column after Done must not become the Done column.
	if _, err := env.deps.BoardUC.AddColumn(ctx, cliHuman(), env.project.ID, "Archive", 99); err != nil {
		t.Fatal(err)
	}
	c := connect(t, env.deps, "kiro")
	task := callTool(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "Ship"})
	res := callTool(t, c, "complete_task", map[string]any{"task_id": task["ID"].(string), "project_id": env.project.ID})

	board, err := env.deps.BoardUC.GetBoard(ctx, env.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range board.Columns {
		if col.ID == res["column_id"] && col.Name != "Done" {
			t.Errorf("complete_task moved the task to %q, want Done", col.Name)
		}
	}
}

func cliHuman() sharedomain.Actor {
	return sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
}

// callToolText invokes a tool and returns its raw text, failing on tool errors.
func callToolText(t *testing.T, c *client.Client, name string, args map[string]any) string {
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
	return text
}

func TestEmptyListsAreJSONArrays(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")
	if got := strings.TrimSpace(callToolText(t, c, "my_tasks", map[string]any{})); got != "[]" {
		t.Errorf("my_tasks with no tasks = %q, want []", got)
	}
	if got := strings.TrimSpace(callToolText(t, c, "list_tasks", map[string]any{"project_id": env.project.ID})); got != "[]" {
		t.Errorf("list_tasks with no tasks = %q, want []", got)
	}
}

func TestErrorsAreNotDuplicated(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")
	task := callTool(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "Move me"})
	msg := callToolErr(t, c, "move_task", map[string]any{"task_id": task["ID"].(string), "target_column_id": "nope"})
	if strings.Count(msg, "not found") != 1 {
		t.Errorf("error repeats itself: %q", msg)
	}
}

func TestGetTaskHidesSoftDeletedTask(t *testing.T) {
	env := newSQLiteEnv(t, "")
	c := connect(t, env.deps, "kiro")
	task := callTool(t, c, "create_task", map[string]any{"project_id": env.project.ID, "title": "Gone"})
	id := task["ID"].(string)
	callTool(t, c, "delete_task", map[string]any{"task_id": id, "confirm": true})
	if msg := callToolErr(t, c, "get_task", map[string]any{"task_id": id}); !strings.Contains(msg, "not found") {
		t.Errorf("get_task on a deleted task = %q, want not found", msg)
	}
}

func TestServerReportsBuildVersion(t *testing.T) {
	env := newSQLiteEnv(t, "")
	env.deps.Version = "9.8.7"
	c, err := client.NewInProcessClient(appmcp.NewServer(env.deps))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	init := mcplib.InitializeRequest{}
	init.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcplib.Implementation{Name: "probe", Version: "1"}
	res, err := c.Initialize(ctx, init)
	if err != nil {
		t.Fatal(err)
	}
	if res.ServerInfo.Version != "9.8.7" {
		t.Errorf("server version = %q, want 9.8.7", res.ServerInfo.Version)
	}
}

// TestServeIODrainsInFlightCallsOnEOF sends a slow tool call and closes stdin
// right away: the call must still complete instead of being cancelled.
func TestServeIODrainsInFlightCallsOnEOF(t *testing.T) {
	s := server.NewMCPServer("probe", "1")
	s.AddTool(mcplib.NewTool("slow"), func(ctx context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		select {
		case <-time.After(300 * time.Millisecond):
			return mcplib.NewToolResultText("finished"), nil
		case <-ctx.Done():
			return mcplib.NewToolResultError(ctx.Err().Error()), nil
		}
	})
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"slow","arguments":{}}}` + "\n")
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- appmcp.ServeIO(context.Background(), s, in, &out, nil) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ServeIO did not return after stdin EOF")
	}
	got := out.String()
	if !strings.Contains(got, "finished") || strings.Contains(got, "context canceled") {
		t.Errorf("in-flight call was not drained:\n%s", got)
	}
}

// syncBuffer is a goroutine-safe bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
