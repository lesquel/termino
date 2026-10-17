package application_test

import (
	"context"
	"testing"

	"github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/events"
)

// conflictingTaskRepo rejects the first numbers it is asked to save, as if
// another process had taken them in the meantime.
type conflictingTaskRepo struct {
	*inMemoryTaskRepo
	conflicts int
}

func (r *conflictingTaskRepo) Save(ctx context.Context, t taskdomain.Task) error {
	if r.conflicts > 0 {
		r.conflicts--
		return taskdomain.ErrDuplicateNumber
	}
	return r.inMemoryTaskRepo.Save(ctx, t)
}

func TestCreateTask_RetriesOnNumberConflict(t *testing.T) {
	repo := &conflictingTaskRepo{inMemoryTaskRepo: newInMemoryTaskRepo(), conflicts: 2}
	uc := application.NewTaskUseCases(repo, newInMemoryCommentRepo(), newInMemoryActivityRepo(), newInMemoryBoardRepo(), events.NewBus())
	task, err := uc.CreateTask(context.Background(), testActor(), defaultInput())
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.Number != 3 {
		t.Errorf("Number = %d, want 3 after two conflicts", task.Number)
	}
	if task.UpdatedBy != "Alice" {
		t.Errorf("UpdatedBy = %q, want the actor name", task.UpdatedBy)
	}
}

func TestUpdateTask_SetsUpdatedBy(t *testing.T) {
	tasks := newInMemoryTaskRepo()
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), newInMemoryActivityRepo(), newInMemoryBoardRepo())
	ctx := context.Background()
	task, err := uc.CreateTask(ctx, testActor(), defaultInput())
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	other := testActor()
	other.Name = "Bob"
	title := "New"
	if err := uc.UpdateTask(ctx, other, application.UpdateTaskInput{TaskID: task.ID, Title: &title}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got, _ := tasks.FindByID(ctx, task.ID)
	if got.UpdatedBy != "Bob" {
		t.Errorf("UpdatedBy = %q, want Bob", got.UpdatedBy)
	}
}
