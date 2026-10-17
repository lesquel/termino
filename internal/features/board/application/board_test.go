package application_test

import (
	"context"
	"errors"
	"testing"

	sharedomain "github.com/lesquel/termino/internal/domain"
	"github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

func testActor() sharedomain.Actor {
	return sharedomain.Actor{ID: "actor-1", Name: "Alice", Kind: sharedomain.ActorKindHuman}
}

func newBoardUC(boardRepo *inMemoryBoardRepo, taskRepo *inMemoryTaskRepo) *application.BoardUseCases {
	return application.NewBoardUseCases(boardRepo, taskRepo)
}

func TestAddColumn(t *testing.T) {
	uc := newBoardUC(newInMemoryBoardRepo(), newInMemoryTaskRepo())
	col, err := uc.AddColumn(context.Background(), testActor(), "proj-1", "In Progress", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if col.ID == "" {
		t.Error("expected non-empty ID")
	}
	if col.Name != "In Progress" {
		t.Errorf("name = %q, want %q", col.Name, "In Progress")
	}
}

func TestRenameColumn(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	uc := newBoardUC(boardRepo, newInMemoryTaskRepo())
	col, _ := uc.AddColumn(context.Background(), testActor(), "proj-1", "Old Name", 0)
	if err := uc.RenameColumn(context.Background(), testActor(), col.ID, "New Name"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	updated, _ := boardRepo.FindColumnByID(context.Background(), col.ID)
	if updated.Name != "New Name" {
		t.Errorf("name = %q, want %q", updated.Name, "New Name")
	}
}

func TestDeleteColumn_Empty(t *testing.T) {
	uc := newBoardUC(newInMemoryBoardRepo(), newInMemoryTaskRepo())
	col, _ := uc.AddColumn(context.Background(), testActor(), "proj-1", "Done", 2)
	if err := uc.DeleteColumn(context.Background(), testActor(), col.ID); err != nil {
		t.Fatalf("delete empty column: %v", err)
	}
}

func TestDeleteColumn_NotEmpty(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	taskRepo := newInMemoryTaskRepo()
	uc := newBoardUC(boardRepo, taskRepo)
	col, _ := uc.AddColumn(context.Background(), testActor(), "proj-1", "Backlog", 0)
	// Add a task to the column.
	_ = taskRepo.Save(context.Background(), taskdomain.Task{
		ID:        "task-1",
		ProjectID: "proj-1",
		ColumnID:  col.ID,
		Title:     "Task",
		Number:    1,
	})
	err := uc.DeleteColumn(context.Background(), testActor(), col.ID)
	if err == nil {
		t.Fatal("expected error for non-empty column, got nil")
	}
}

func TestSetWipLimit(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	uc := newBoardUC(boardRepo, newInMemoryTaskRepo())
	col, _ := uc.AddColumn(context.Background(), testActor(), "proj-1", "Progress", 1)
	limit := 3
	if err := uc.SetWipLimit(context.Background(), testActor(), col.ID, &limit); err != nil {
		t.Fatalf("set wip limit: %v", err)
	}
	updated, _ := boardRepo.FindColumnByID(context.Background(), col.ID)
	if updated.WIPLimit == nil || *updated.WIPLimit != 3 {
		t.Errorf("wip limit = %v, want 3", updated.WIPLimit)
	}
}

func TestGetBoard(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	taskRepo := newInMemoryTaskRepo()
	uc := newBoardUC(boardRepo, taskRepo)
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{
		ID: "col-1", ProjectID: "proj-1", Name: "Backlog", Position: 0,
	})
	_ = taskRepo.Save(context.Background(), taskdomain.Task{
		ID: "task-1", ProjectID: "proj-1", ColumnID: "col-1", Title: "T1", Number: 1,
	})
	board, err := uc.GetBoard(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(board.Columns) != 1 {
		t.Errorf("columns = %d, want 1", len(board.Columns))
	}
	if len(board.Tasks["col-1"]) != 1 {
		t.Errorf("tasks in col-1 = %d, want 1", len(board.Tasks["col-1"]))
	}
}

func TestWIPLimitEnforced(t *testing.T) {
	limit := 1
	col := boarddomain.Column{ID: "col-1", WIPLimit: &limit}
	board := boarddomain.Board{Tasks: map[string][]string{"col-1": {"task-existing"}}}
	err := boarddomain.CanAddToColumn(board, col)
	if err == nil {
		t.Fatal("expected WIP limit error, got nil")
	}
}

func TestGetBoardWithTasks_GroupsFullTasksByColumnInBoardOrder(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	taskRepo := newInMemoryTaskRepo()
	uc := newBoardUC(boardRepo, taskRepo)
	ctx := context.Background()
	col, _ := uc.AddColumn(ctx, testActor(), "proj-1", "Backlog", 0)
	_ = taskRepo.Save(ctx, taskdomain.Task{ID: "t1", ProjectID: "proj-1", Number: 1, Title: "First", ColumnID: col.ID, Position: 1})
	_ = taskRepo.Save(ctx, taskdomain.Task{ID: "t2", ProjectID: "proj-1", Number: 2, Title: "Second", ColumnID: col.ID, Position: 2})

	board, tasks, err := uc.GetBoardWithTasks(ctx, "proj-1")
	if err != nil {
		t.Fatalf("GetBoardWithTasks: %v", err)
	}
	ids := board.Tasks[col.ID]
	full := tasks[col.ID]
	if len(ids) != 2 || len(full) != 2 {
		t.Fatalf("got %d ids and %d tasks, want 2 and 2", len(ids), len(full))
	}
	for i := range ids {
		if ids[i] != full[i].ID {
			t.Errorf("index %d: id %q does not match task %q", i, ids[i], full[i].ID)
		}
	}
}

func TestAddColumnRejectsDuplicateNameIgnoringCase(t *testing.T) {
	uc := newBoardUC(newInMemoryBoardRepo(), newInMemoryTaskRepo())
	ctx := context.Background()
	if _, err := uc.AddColumn(ctx, testActor(), "proj-1", "Review", 0); err != nil {
		t.Fatal(err)
	}
	_, err := uc.AddColumn(ctx, testActor(), "proj-1", "  review ", 1)
	if !errors.Is(err, application.ErrDuplicateColumnName) {
		t.Fatalf("err = %v, want ErrDuplicateColumnName", err)
	}
	// Same name in another project is fine.
	if _, err := uc.AddColumn(ctx, testActor(), "proj-2", "Review", 0); err != nil {
		t.Errorf("other project: %v", err)
	}
}

func TestRenameColumnRejectsDuplicateNameIgnoringCase(t *testing.T) {
	uc := newBoardUC(newInMemoryBoardRepo(), newInMemoryTaskRepo())
	ctx := context.Background()
	a, _ := uc.AddColumn(ctx, testActor(), "proj-1", "Todo", 0)
	if _, err := uc.AddColumn(ctx, testActor(), "proj-1", "Done", 1); err != nil {
		t.Fatal(err)
	}
	if err := uc.RenameColumn(ctx, testActor(), a.ID, "DONE"); !errors.Is(err, application.ErrDuplicateColumnName) {
		t.Fatalf("err = %v, want ErrDuplicateColumnName", err)
	}
	// Renaming a column to a different case of its own name is allowed.
	if err := uc.RenameColumn(ctx, testActor(), a.ID, "TODO"); err != nil {
		t.Errorf("self rename: %v", err)
	}
}
