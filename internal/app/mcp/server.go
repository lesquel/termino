// Package mcp implements the MCP server with 13 tools for the Termino application.
// All tool handlers call application use cases only — never the DB directly.
// Never print to stdout from this package (would break the stdio protocol).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	sharedomain "github.com/lesquel/termino/internal/domain"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	agentapp "github.com/lesquel/termino/internal/features/agent/application"
	agentdomain "github.com/lesquel/termino/internal/features/agent/domain"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// Deps holds the application dependencies for the MCP server.
type Deps struct {
	ProjectUC *projectapp.ProjectUseCases
	BoardUC   *boardapp.BoardUseCases
	TaskUC    *taskapp.TaskUseCases
	AgentUC   *agentapp.AgentUseCases
	Logger    *slog.Logger
	// ActorOverride is set from --actor flag before serving.
	ActorOverride string
	// ReadOnly disables the 9 write tools when true.
	ReadOnly bool
}

// NewServer creates and configures the MCP server with all 13 tools registered.
func NewServer(deps Deps) *server.MCPServer {
	s := server.NewMCPServer(
		"termino",
		"1.0.0",
	)

	// Parse --actor and --read-only from os.Args before the server loop.
	for i, arg := range os.Args {
		if arg == "--actor" && i+1 < len(os.Args) {
			deps.ActorOverride = os.Args[i+1]
		}
		if arg == "--read-only" {
			deps.ReadOnly = true
		}
	}

	// Register the 5 read tools (always available).
	s.AddTool(listProjectsTool(), makeListProjectsHandler(deps))
	s.AddTool(getBoardTool(), makeGetBoardHandler(deps))
	s.AddTool(listTasksTool(), makeListTasksHandler(deps))
	s.AddTool(getTaskTool(), makeGetTaskHandler(deps))
	s.AddTool(myTasksTool(), makeMyTasksHandler(deps))

	if !deps.ReadOnly {
		// Register the 8 write tools.
		s.AddTool(createTaskTool(), makeCreateTaskHandler(deps))
		s.AddTool(updateTaskTool(), makeUpdateTaskHandler(deps))
		s.AddTool(moveTaskTool(), makeMoveTaskHandler(deps))
		s.AddTool(assignTaskTool(), makeAssignTaskHandler(deps))
		s.AddTool(claimTaskTool(), makeClaimTaskHandler(deps))
		s.AddTool(completeTaskTool(), makeCompleteTaskHandler(deps))
		s.AddTool(addCommentTool(), makeAddCommentHandler(deps))
		s.AddTool(deleteTaskTool(), makeDeleteTaskHandler(deps))
	}

	return s
}

// Serve starts the MCP server using stdio transport.
// Never prints to stdout.
func Serve(s *server.MCPServer, logger *slog.Logger) error {
	if logger != nil {
		logger.Info("termino MCP server starting", "transport", "stdio")
	}
	return server.ServeStdio(s)
}

// --- Helper ---

func resolveActor(deps Deps, clientName string) sharedomain.Actor {
	name := deps.ActorOverride
	if name == "" {
		name = clientName
	}
	if name == "" {
		name = "agent"
	}
	return sharedomain.Actor{
		ID:   name,
		Name: name,
		Kind: sharedomain.ActorKindAgent,
	}
}

func textResult(v any) (*mcplib.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcplib.NewToolResultText(string(b)), nil
}

func errResult(err error) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultError(err.Error()), nil
}

func updatePresence(ctx context.Context, deps Deps, actor sharedomain.Actor) {
	if deps.AgentUC == nil {
		return
	}
	_ = deps.AgentUC.UpdatePresence(ctx, agentdomain.AgentSession{
		AgentID:   actor.ID,
		AgentName: actor.Name,
	})
}

// --- Tool definitions ---

func listProjectsTool() mcplib.Tool {
	return mcplib.NewTool("list_projects",
		mcplib.WithDescription("List all non-archived projects"),
	)
}

func getBoardTool() mcplib.Tool {
	return mcplib.NewTool("get_board",
		mcplib.WithDescription("Get the Kanban board with columns and tasks for a project"),
		mcplib.WithString("project_id",
			mcplib.Required(),
			mcplib.Description("The project ID"),
		),
	)
}

func listTasksTool() mcplib.Tool {
	return mcplib.NewTool("list_tasks",
		mcplib.WithDescription("List tasks with optional filters"),
		mcplib.WithString("project_id", mcplib.Description("Filter by project ID")),
		mcplib.WithString("assignee_id", mcplib.Description("Filter by assignee ID")),
		mcplib.WithString("column_id", mcplib.Description("Filter by column ID")),
		mcplib.WithString("label_ids", mcplib.Description("Comma-separated label IDs")),
		mcplib.WithString("search_text", mcplib.Description("Full-text search")),
	)
}

func getTaskTool() mcplib.Tool {
	return mcplib.NewTool("get_task",
		mcplib.WithDescription("Get full task details including comments and activity"),
		mcplib.WithString("task_id",
			mcplib.Required(),
			mcplib.Description("The task ID"),
		),
	)
}

func myTasksTool() mcplib.Tool {
	return mcplib.NewTool("my_tasks",
		mcplib.WithDescription("List tasks assigned to the calling agent"),
		mcplib.WithString("actor_id", mcplib.Description("Actor ID (defaults to calling agent)")),
	)
}

func createTaskTool() mcplib.Tool {
	return mcplib.NewTool("create_task",
		mcplib.WithDescription("Create a new task on the board"),
		mcplib.WithString("project_id", mcplib.Required(), mcplib.Description("Project ID")),
		mcplib.WithString("title", mcplib.Required(), mcplib.Description("Task title")),
		mcplib.WithString("description", mcplib.Description("Task description")),
		mcplib.WithString("priority", mcplib.Description("Priority: low, medium, high, critical")),
		mcplib.WithString("column_id", mcplib.Description("Target column ID")),
		mcplib.WithString("assignee_id", mcplib.Description("Assignee member ID")),
	)
}

func updateTaskTool() mcplib.Tool {
	return mcplib.NewTool("update_task",
		mcplib.WithDescription("Update task fields"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithString("title", mcplib.Description("New title")),
		mcplib.WithString("description", mcplib.Description("New description")),
		mcplib.WithString("priority", mcplib.Description("New priority")),
	)
}

func moveTaskTool() mcplib.Tool {
	return mcplib.NewTool("move_task",
		mcplib.WithDescription("Move a task to a different column"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithString("target_column_id", mcplib.Required(), mcplib.Description("Target column ID")),
		mcplib.WithNumber("position", mcplib.Description("Fractional position in column (optional)")),
	)
}

func assignTaskTool() mcplib.Tool {
	return mcplib.NewTool("assign_task",
		mcplib.WithDescription("Assign or unassign a task"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithString("assignee_id", mcplib.Description("Assignee ID; omit to unassign")),
	)
}

func claimTaskTool() mcplib.Tool {
	return mcplib.NewTool("claim_task",
		mcplib.WithDescription("Atomically assign task to calling agent and move to in-progress column"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithString("project_id", mcplib.Required(), mcplib.Description("Project ID")),
	)
}

func completeTaskTool() mcplib.Tool {
	return mcplib.NewTool("complete_task",
		mcplib.WithDescription("Move a task to the done column (last column by position)"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithString("project_id", mcplib.Required(), mcplib.Description("Project ID")),
	)
}

func addCommentTool() mcplib.Tool {
	return mcplib.NewTool("add_comment",
		mcplib.WithDescription("Add a comment to a task"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithString("body", mcplib.Required(), mcplib.Description("Comment text")),
	)
}

func deleteTaskTool() mcplib.Tool {
	return mcplib.NewTool("delete_task",
		mcplib.WithDescription("Soft-delete a task (requires confirm: true)"),
		mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID")),
		mcplib.WithBoolean("confirm", mcplib.Required(), mcplib.Description("Must be true to confirm deletion")),
	)
}

// --- Handlers ---

func makeListProjectsHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		projects, err := deps.ProjectUC.ListProjects(ctx)
		if err != nil {
			return errResult(err)
		}
		return textResult(projects)
	}
}

func makeGetBoardHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		projectID, err := req.RequireString("project_id")
		if err != nil {
			return errResult(fmt.Errorf("missing project_id: %w", err))
		}
		board, err := deps.BoardUC.GetBoard(ctx, projectID)
		if err != nil {
			return errResult(err)
		}
		return textResult(board)
	}
}

func makeListTasksHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		filter := taskdomain.TaskFilter{
			ProjectID:  req.GetString("project_id", ""),
			AssigneeID: req.GetString("assignee_id", ""),
			ColumnID:   req.GetString("column_id", ""),
			SearchText: req.GetString("search_text", ""),
		}
		if labelsRaw := req.GetString("label_ids", ""); labelsRaw != "" {
			filter.LabelIDs = strings.Split(labelsRaw, ",")
		}
		tasks, err := deps.TaskUC.ListTasks(ctx, filter)
		if err != nil {
			return errResult(err)
		}
		return textResult(tasks)
	}
}

func makeGetTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		task, err := deps.TaskUC.FindTaskByID(ctx, taskID)
		if err != nil {
			return errResult(err)
		}
		comments, _ := deps.TaskUC.ListCommentsByTask(ctx, taskID)
		activity, _ := deps.TaskUC.ListActivityByTask(ctx, taskID)
		result := map[string]any{
			"task":     task,
			"comments": comments,
			"activity": activity,
		}
		return textResult(result)
	}
}

func makeMyTasksHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		actorID := req.GetString("actor_id", "")
		if actorID == "" {
			actorID = deps.ActorOverride
		}
		if actorID == "" {
			actorID = "agent"
		}
		tasks, err := deps.TaskUC.MyTasks(ctx, actorID)
		if err != nil {
			return errResult(err)
		}
		return textResult(tasks)
	}
}

func makeCreateTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		projectID, err := req.RequireString("project_id")
		if err != nil {
			return errResult(fmt.Errorf("missing project_id: %w", err))
		}
		title, err := req.RequireString("title")
		if err != nil {
			return errResult(fmt.Errorf("missing title: %w", err))
		}
		actor := resolveActor(deps, "")
		priority := taskdomain.Priority(req.GetString("priority", "medium"))
		input := taskapp.CreateTaskInput{
			ProjectID:   projectID,
			Title:       title,
			Description: req.GetString("description", ""),
			ColumnID:    req.GetString("column_id", ""),
			Priority:    priority,
		}
		if aID := req.GetString("assignee_id", ""); aID != "" {
			input.AssigneeID = &aID
		}
		task, err := deps.TaskUC.CreateTask(ctx, actor, input)
		if err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(task)
	}
}

func makeUpdateTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		actor := resolveActor(deps, "")
		input := taskapp.UpdateTaskInput{TaskID: taskID}
		if v := req.GetString("title", ""); v != "" {
			input.Title = &v
		}
		if v := req.GetString("description", ""); v != "" {
			input.Description = &v
		}
		if v := req.GetString("priority", ""); v != "" {
			p := taskdomain.Priority(v)
			input.Priority = &p
		}
		if err := deps.TaskUC.UpdateTask(ctx, actor, input); err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(map[string]string{"status": "updated", "task_id": taskID})
	}
}

func makeMoveTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		colID, err := req.RequireString("target_column_id")
		if err != nil {
			return errResult(fmt.Errorf("missing target_column_id: %w", err))
		}
		position := req.GetFloat("position", 0)
		actor := resolveActor(deps, "")
		if err := deps.TaskUC.MoveTask(ctx, actor, taskID, colID, position); err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(map[string]string{"status": "moved", "task_id": taskID})
	}
}

func makeAssignTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		actor := resolveActor(deps, "")
		var assigneeID *string
		if v := req.GetString("assignee_id", ""); v != "" {
			assigneeID = &v
		}
		if err := deps.TaskUC.AssignTask(ctx, actor, taskID, assigneeID); err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(map[string]string{"status": "assigned", "task_id": taskID})
	}
}

func makeClaimTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		projectID, err := req.RequireString("project_id")
		if err != nil {
			return errResult(fmt.Errorf("missing project_id: %w", err))
		}
		actor := resolveActor(deps, "")

		// Find the in-progress column (position 1 or named "In Progress"/"En Progreso").
		board, err := deps.BoardUC.GetBoard(ctx, projectID)
		if err != nil {
			return errResult(err)
		}
		inProgressCol := findInProgressColumn(board.Columns)
		if inProgressCol == "" {
			return errResult(fmt.Errorf("could not find in-progress column for project %s", projectID))
		}

		// Assign task to agent.
		if err := deps.TaskUC.AssignTask(ctx, actor, taskID, &actor.ID); err != nil {
			return errResult(err)
		}
		// Move to in-progress.
		if err := deps.TaskUC.MoveTask(ctx, actor, taskID, inProgressCol, 0); err != nil {
			return errResult(err)
		}
		// Update agent presence.
		updatePresence(ctx, deps, actor)
		if deps.AgentUC != nil {
			_ = deps.AgentUC.ClaimTask(ctx, actor.ID, taskID)
		}
		return textResult(map[string]string{"status": "claimed", "task_id": taskID, "column_id": inProgressCol})
	}
}

func makeCompleteTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		projectID, err := req.RequireString("project_id")
		if err != nil {
			return errResult(fmt.Errorf("missing project_id: %w", err))
		}
		actor := resolveActor(deps, "")

		board, err := deps.BoardUC.GetBoard(ctx, projectID)
		if err != nil {
			return errResult(err)
		}
		// Done column = last column by position.
		doneCol := findDoneColumn(board.Columns)
		if doneCol == "" {
			return errResult(fmt.Errorf("could not find done column for project %s", projectID))
		}
		if err := deps.TaskUC.MoveTask(ctx, actor, taskID, doneCol, 0); err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(map[string]string{"status": "completed", "task_id": taskID, "column_id": doneCol})
	}
}

func makeAddCommentHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		body, err := req.RequireString("body")
		if err != nil {
			return errResult(fmt.Errorf("missing body: %w", err))
		}
		actor := resolveActor(deps, "")
		comment, err := deps.TaskUC.AddComment(ctx, actor, taskID, body)
		if err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(comment)
	}
}

func makeDeleteTaskHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errResult(fmt.Errorf("missing task_id: %w", err))
		}
		confirm := req.GetBool("confirm", false)
		if !confirm {
			// Get the task ref for the error message.
			t, findErr := deps.TaskUC.FindTaskByID(ctx, taskID)
			ref := taskID
			if findErr == nil {
				ref = fmt.Sprintf("TER-%s", strconv.Itoa(t.Number))
			}
			return errResult(fmt.Errorf("Pass confirm: true to delete task %s", ref))
		}
		actor := resolveActor(deps, "")
		if err := deps.TaskUC.DeleteTask(ctx, actor, taskID, confirm); err != nil {
			return errResult(err)
		}
		updatePresence(ctx, deps, actor)
		return textResult(map[string]string{"status": "deleted", "task_id": taskID})
	}
}

// findInProgressColumn returns the ID of the in-progress column:
// the column at position 1, or the first whose name contains "progress" (case-insensitive).
func findInProgressColumn(cols []boarddomain.Column) string {
	for _, c := range cols {
		lower := strings.ToLower(c.Name)
		if strings.Contains(lower, "progress") {
			return c.ID
		}
	}
	// Fallback: position 1.
	for _, c := range cols {
		if c.Position == 1 {
			return c.ID
		}
	}
	if len(cols) > 1 {
		return cols[1].ID
	}
	return ""
}

// findDoneColumn returns the ID of the last column by position.
func findDoneColumn(cols []boarddomain.Column) string {
	if len(cols) == 0 {
		return ""
	}
	last := cols[0]
	for _, c := range cols {
		if c.Position > last.Position {
			last = c
		}
	}
	return last.ID
}
