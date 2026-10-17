package mcp_test

import (
	"context"
	"strings"
	"testing"

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
