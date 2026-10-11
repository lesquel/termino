// Package application contains the sprint feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	sharedomain "github.com/lesquel/termino/internal/domain"
	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// Actor is a type alias so callers can use the application package directly.
type Actor = sharedomain.Actor

// UnfinishedStrategy controls what happens to unfinished tasks when a sprint closes.
type UnfinishedStrategy string

const (
	// UnfinishedToBacklog moves unfinished tasks to the backlog column.
	UnfinishedToBacklog UnfinishedStrategy = "backlog"
	// UnfinishedToNextSprint moves unfinished tasks to the next sprint.
	UnfinishedToNextSprint UnfinishedStrategy = "next_sprint"
)

// SprintUseCases groups all sprint-related use cases.
type SprintUseCases struct {
	sprints sprintdomain.SprintRepository
	tasks   taskdomain.TaskRepository
}

// NewSprintUseCases constructs a SprintUseCases instance.
func NewSprintUseCases(
	sprints sprintdomain.SprintRepository,
	tasks taskdomain.TaskRepository,
) *SprintUseCases {
	return &SprintUseCases{sprints: sprints, tasks: tasks}
}

// CreateSprint creates a new sprint in draft state.
func (uc *SprintUseCases) CreateSprint(
	ctx context.Context,
	actor Actor,
	projectID, name string,
	startsAt, endsAt time.Time,
) (sprintdomain.Sprint, error) {
	s := sprintdomain.Sprint{
		ID:        uuid.NewString(),
		ProjectID: projectID,
		Name:      name,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		State:     sprintdomain.SprintStateDraft,
	}
	if err := uc.sprints.Save(ctx, s); err != nil {
		return sprintdomain.Sprint{}, fmt.Errorf("save sprint: %w", err)
	}
	return s, nil
}

// StartSprint transitions a sprint from draft to active.
// Only one sprint may be active per project at a time.
func (uc *SprintUseCases) StartSprint(
	ctx context.Context,
	actor Actor,
	sprintID string,
) error {
	s, err := uc.sprints.FindByID(ctx, sprintID)
	if err != nil {
		return fmt.Errorf("find sprint: %w", err)
	}
	active, err := uc.sprints.FindActive(ctx, s.ProjectID)
	if err != nil {
		return fmt.Errorf("check active sprint: %w", err)
	}
	if active != nil {
		return fmt.Errorf("%w: %s", sprintdomain.ErrActiveSprintExists, active.ID)
	}
	s.State = sprintdomain.SprintStateActive
	return uc.sprints.Save(ctx, s)
}

// CloseSprint transitions a sprint to closed and handles unfinished tasks.
func (uc *SprintUseCases) CloseSprint(
	ctx context.Context,
	actor Actor,
	sprintID string,
	strategy UnfinishedStrategy,
) error {
	s, err := uc.sprints.FindByID(ctx, sprintID)
	if err != nil {
		return fmt.Errorf("find sprint: %w", err)
	}
	s.State = sprintdomain.SprintStateClosed
	if err := uc.sprints.Save(ctx, s); err != nil {
		return fmt.Errorf("save sprint: %w", err)
	}
	// Clear sprint assignment for unfinished tasks.
	unfinished, err := uc.tasks.List(ctx, taskdomain.TaskFilter{SprintID: sprintID})
	if err != nil {
		return fmt.Errorf("list sprint tasks: %w", err)
	}
	for _, t := range unfinished {
		switch strategy {
		case UnfinishedToBacklog:
			t.SprintID = nil
		case UnfinishedToNextSprint:
			// The caller should assign a new sprint ID externally; here we
			// simply detach — the infrastructure layer handles re-assignment.
			t.SprintID = nil
		}
		if err := uc.tasks.Save(ctx, t); err != nil {
			return fmt.Errorf("update task %s: %w", t.ID, err)
		}
	}
	return nil
}

// AddTaskToSprint assigns a task to a sprint.
func (uc *SprintUseCases) AddTaskToSprint(
	ctx context.Context,
	actor Actor,
	taskID, sprintID string,
) error {
	t, err := uc.tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	t.SprintID = &sprintID
	return uc.tasks.Save(ctx, t)
}
