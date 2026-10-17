package application_test

import (
	"context"
	"testing"

	sharedomain "github.com/lesquel/termino/internal/domain"
	"github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	"github.com/lesquel/termino/internal/platform/events"
)

func testActor() sharedomain.Actor {
	return sharedomain.Actor{ID: "actor-1", Name: "Alice", Kind: sharedomain.ActorKindHuman}
}

func newTaskUC(tasks *inMemoryTaskRepo, comments *inMemoryCommentRepo, activities *inMemoryActivityRepo, board *inMemoryBoardRepo) *application.TaskUseCases {
	return application.NewTaskUseCases(tasks, comments, activities, board, events.NewBus())
}

func defaultInput() application.CreateTaskInput {
	return application.CreateTaskInput{
		ProjectID: "proj-1",
		Title:     "My Task",
		ColumnID:  "col-1",
		Priority:  taskdomain.PriorityMedium,
	}
}

func TestCreateTask_HappyPath(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	activities := newInMemoryActivityRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), activities, newInMemoryBoardRepo())
	task, err := uc.CreateTask(context.Background(), testActor(), defaultInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID == "" {
		t.Error("expected non-empty task ID")
	}
	if task.Number != 1 {
		t.Errorf("number = %d, want 1", task.Number)
	}
	// Verify activity was recorded.
	acts, _ := activities.ListByTask(context.Background(), task.ID)
	if len(acts) == 0 {
		t.Error("expected at least one activity entry")
	}
	if acts[0].Action != "created" {
		t.Errorf("action = %q, want %q", acts[0].Action, "created")
	}
}

func TestCreateTask_DefaultsToFirstColumn(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{ID: "col-done", ProjectID: "proj-1", Name: "Done", Position: 2})
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{ID: "col-backlog", ProjectID: "proj-1", Name: "Backlog", Position: 0})
	uc := newTaskUC(newInMemoryTaskRepo(), newInMemoryCommentRepo(), newInMemoryActivityRepo(), boardRepo)

	in := defaultInput()
	in.ColumnID = ""
	task, err := uc.CreateTask(context.Background(), testActor(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ColumnID != "col-backlog" {
		t.Errorf("column = %q, want first column %q", task.ColumnID, "col-backlog")
	}
}

func TestCreateTask_NumbersIncrement(t *testing.T) {
	uc := newTaskUC(newInMemoryTaskRepo(), newInMemoryCommentRepo(), newInMemoryActivityRepo(), newInMemoryBoardRepo())
	t1, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	t2, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	if t2.Number != t1.Number+1 {
		t.Errorf("numbers not incrementing: %d -> %d", t1.Number, t2.Number)
	}
}

func TestUpdateTask(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	activities := newInMemoryActivityRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), activities, newInMemoryBoardRepo())
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	newTitle := "Updated Title"
	if err := uc.UpdateTask(context.Background(), testActor(), application.UpdateTaskInput{
		TaskID: task.ID,
		Title:  &newTitle,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, _ := tasks.FindByID(context.Background(), task.ID)
	if updated.Title != newTitle {
		t.Errorf("title = %q, want %q", updated.Title, newTitle)
	}
	acts, _ := activities.ListByTask(context.Background(), task.ID)
	hasUpdated := false
	for _, a := range acts {
		if a.Action == "updated" {
			hasUpdated = true
		}
	}
	if !hasUpdated {
		t.Error("expected 'updated' activity")
	}
}

func TestMoveTask(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	tasks := newInMemoryTaskRepo()
	activities := newInMemoryActivityRepo()
	// Set up columns.
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{ID: "col-1", ProjectID: "proj-1", Name: "Backlog"})
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{ID: "col-2", ProjectID: "proj-1", Name: "In Progress"})
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), activities, boardRepo)
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	if err := uc.MoveTask(context.Background(), testActor(), task.ID, "col-2", 1.0); err != nil {
		t.Fatalf("move: %v", err)
	}
	updated, _ := tasks.FindByID(context.Background(), task.ID)
	if updated.ColumnID != "col-2" {
		t.Errorf("columnID = %q, want %q", updated.ColumnID, "col-2")
	}
	acts, _ := activities.ListByTask(context.Background(), task.ID)
	hasMoved := false
	for _, a := range acts {
		if a.Action == "moved" {
			hasMoved = true
		}
	}
	if !hasMoved {
		t.Error("expected 'moved' activity")
	}
}

func TestMoveTask_WIPLimitExceeded(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	tasks := newInMemoryTaskRepo()
	limit := 1
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{
		ID: "col-1", ProjectID: "proj-1", Name: "Backlog",
	})
	_ = boardRepo.SaveColumn(context.Background(), boarddomain.Column{
		ID: "col-2", ProjectID: "proj-1", Name: "In Progress", WIPLimit: &limit,
	})
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), newInMemoryActivityRepo(), boardRepo)
	// Create two tasks and move the first one in.
	t1, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	t2, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	if err := uc.MoveTask(context.Background(), testActor(), t1.ID, "col-2", 1.0); err != nil {
		t.Fatalf("first move: %v", err)
	}
	// Second move should fail WIP limit.
	err := uc.MoveTask(context.Background(), testActor(), t2.ID, "col-2", 2.0)
	if err == nil {
		t.Fatal("expected WIP limit error, got nil")
	}
}

func TestDeleteTask_RequiresConfirm(t *testing.T) {
	uc := newTaskUC(newInMemoryTaskRepo(), newInMemoryCommentRepo(), newInMemoryActivityRepo(), newInMemoryBoardRepo())
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	err := uc.DeleteTask(context.Background(), testActor(), task.ID, false)
	if err == nil {
		t.Fatal("expected ErrConfirmRequired, got nil")
	}
}

func TestDeleteTask_SoftDeletes(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	activities := newInMemoryActivityRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), activities, newInMemoryBoardRepo())
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	if err := uc.DeleteTask(context.Background(), testActor(), task.ID, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deleted, _ := tasks.FindByID(context.Background(), task.ID)
	if deleted.DeletedAt == nil {
		t.Error("expected DeletedAt to be set")
	}
	acts, _ := activities.ListByTask(context.Background(), task.ID)
	hasDeleted := false
	for _, a := range acts {
		if a.Action == "deleted" {
			hasDeleted = true
		}
	}
	if !hasDeleted {
		t.Error("expected 'deleted' activity")
	}
}

func TestRestoreTask(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	activities := newInMemoryActivityRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), activities, newInMemoryBoardRepo())
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	_ = uc.DeleteTask(context.Background(), testActor(), task.ID, true)
	if err := uc.RestoreTask(context.Background(), testActor(), task.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, _ := tasks.FindByID(context.Background(), task.ID)
	if restored.DeletedAt != nil {
		t.Error("expected DeletedAt to be nil after restore")
	}
}

func TestAssignTask(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	activities := newInMemoryActivityRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), activities, newInMemoryBoardRepo())
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	assignee := "member-1"
	if err := uc.AssignTask(context.Background(), testActor(), task.ID, &assignee); err != nil {
		t.Fatalf("assign: %v", err)
	}
	updated, _ := tasks.FindByID(context.Background(), task.ID)
	if updated.AssigneeID == nil || *updated.AssigneeID != assignee {
		t.Errorf("assigneeID = %v, want %q", updated.AssigneeID, assignee)
	}
	acts, _ := activities.ListByTask(context.Background(), task.ID)
	hasAssigned := false
	for _, a := range acts {
		if a.Action == "assigned" {
			hasAssigned = true
		}
	}
	if !hasAssigned {
		t.Error("expected 'assigned' activity")
	}
}

func TestAddComment(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	comments := newInMemoryCommentRepo()
	uc := newTaskUC(tasks, comments, newInMemoryActivityRepo(), newInMemoryBoardRepo())
	task, _ := uc.CreateTask(context.Background(), testActor(), defaultInput())
	c, err := uc.AddComment(context.Background(), testActor(), task.ID, "Great work!")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if c.Body != "Great work!" {
		t.Errorf("body = %q", c.Body)
	}
	list, _ := comments.ListByTask(context.Background(), task.ID)
	if len(list) != 1 {
		t.Errorf("comment count = %d, want 1", len(list))
	}
}

func TestMyTasks(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), newInMemoryActivityRepo(), newInMemoryBoardRepo())
	assignee := "member-1"
	input := defaultInput()
	input.AssigneeID = &assignee
	_, _ = uc.CreateTask(context.Background(), testActor(), input)
	_, _ = uc.CreateTask(context.Background(), testActor(), defaultInput()) // not assigned
	myTasks, err := uc.MyTasks(context.Background(), assignee)
	if err != nil {
		t.Fatalf("my tasks: %v", err)
	}
	if len(myTasks) != 1 {
		t.Errorf("task count = %d, want 1", len(myTasks))
	}
}
