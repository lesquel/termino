package application_test

import (
	"context"
	"testing"
	"time"

	sharedomain "github.com/lesquel/termino/internal/domain"
	"github.com/lesquel/termino/internal/features/sprint/application"
	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

func testActor() sharedomain.Actor {
	return sharedomain.Actor{ID: "actor-1", Name: "Alice", Kind: sharedomain.ActorKindHuman}
}

func newSprintUC(sprints *inMemorySprintRepo, tasks *inMemoryTaskRepo) *application.SprintUseCases {
	return application.NewSprintUseCases(sprints, tasks)
}

func defaultSprintTimes() (time.Time, time.Time) {
	now := time.Now().UTC()
	return now, now.Add(14 * 24 * time.Hour)
}

func TestCreateSprint(t *testing.T) {
	uc := newSprintUC(newInMemorySprintRepo(), newInMemoryTaskRepo())
	starts, ends := defaultSprintTimes()
	s, err := uc.CreateSprint(context.Background(), testActor(), "proj-1", "Sprint 1", starts, ends)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.State != sprintdomain.SprintStateDraft {
		t.Errorf("state = %q, want draft", s.State)
	}
}

func TestStartSprint(t *testing.T) {
	sprints := newInMemorySprintRepo()
	uc := newSprintUC(sprints, newInMemoryTaskRepo())
	starts, ends := defaultSprintTimes()
	s, _ := uc.CreateSprint(context.Background(), testActor(), "proj-1", "Sprint 1", starts, ends)
	if err := uc.StartSprint(context.Background(), testActor(), s.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	updated, _ := sprints.FindByID(context.Background(), s.ID)
	if updated.State != sprintdomain.SprintStateActive {
		t.Errorf("state = %q, want active", updated.State)
	}
}

func TestStartSprint_OnlyOneActive(t *testing.T) {
	uc := newSprintUC(newInMemorySprintRepo(), newInMemoryTaskRepo())
	starts, ends := defaultSprintTimes()
	s1, _ := uc.CreateSprint(context.Background(), testActor(), "proj-1", "Sprint 1", starts, ends)
	s2, _ := uc.CreateSprint(context.Background(), testActor(), "proj-1", "Sprint 2", starts, ends)
	_ = uc.StartSprint(context.Background(), testActor(), s1.ID)
	err := uc.StartSprint(context.Background(), testActor(), s2.ID)
	if err == nil {
		t.Fatal("expected error when starting a second active sprint, got nil")
	}
}

func TestCloseSprint(t *testing.T) {
	sprints := newInMemorySprintRepo()
	tasks := newInMemoryTaskRepo()
	uc := newSprintUC(sprints, tasks)
	starts, ends := defaultSprintTimes()
	s, _ := uc.CreateSprint(context.Background(), testActor(), "proj-1", "Sprint 1", starts, ends)
	_ = uc.StartSprint(context.Background(), testActor(), s.ID)
	// Add an unfinished task.
	sprintID := s.ID
	_ = tasks.Save(context.Background(), taskdomain.Task{
		ID: "task-1", ProjectID: "proj-1", SprintID: &sprintID, ColumnID: "col-1", Number: 1, Title: "T1",
	})
	if err := uc.CloseSprint(context.Background(), testActor(), s.ID, application.UnfinishedToBacklog); err != nil {
		t.Fatalf("close: %v", err)
	}
	closed, _ := sprints.FindByID(context.Background(), s.ID)
	if closed.State != sprintdomain.SprintStateClosed {
		t.Errorf("state = %q, want closed", closed.State)
	}
	// Unfinished task should have been detached from the sprint.
	task, _ := tasks.FindByID(context.Background(), "task-1")
	if task.SprintID != nil {
		t.Errorf("expected task SprintID to be nil after close, got %v", task.SprintID)
	}
}

func TestAddTaskToSprint(t *testing.T) {
	sprints := newInMemorySprintRepo()
	tasks := newInMemoryTaskRepo()
	uc := newSprintUC(sprints, tasks)
	starts, ends := defaultSprintTimes()
	s, _ := uc.CreateSprint(context.Background(), testActor(), "proj-1", "Sprint 1", starts, ends)
	_ = tasks.Save(context.Background(), taskdomain.Task{
		ID: "task-1", ProjectID: "proj-1", ColumnID: "col-1", Number: 1, Title: "T1",
	})
	if err := uc.AddTaskToSprint(context.Background(), testActor(), "task-1", s.ID); err != nil {
		t.Fatalf("add task to sprint: %v", err)
	}
	updated, _ := tasks.FindByID(context.Background(), "task-1")
	if updated.SprintID == nil || *updated.SprintID != s.ID {
		t.Errorf("sprintID = %v, want %q", updated.SprintID, s.ID)
	}
}
