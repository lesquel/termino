package application_test

import (
	"context"
	"errors"
	"sync"
	"time"

	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// --- in-memory sprint repository ---

type inMemorySprintRepo struct {
	mu      sync.RWMutex
	sprints map[string]sprintdomain.Sprint
}

func newInMemorySprintRepo() *inMemorySprintRepo {
	return &inMemorySprintRepo{sprints: make(map[string]sprintdomain.Sprint)}
}

func (r *inMemorySprintRepo) Save(_ context.Context, s sprintdomain.Sprint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sprints[s.ID] = s
	return nil
}

func (r *inMemorySprintRepo) FindByID(_ context.Context, id string) (sprintdomain.Sprint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sprints[id]
	if !ok {
		return sprintdomain.Sprint{}, errors.New("sprint not found")
	}
	return s, nil
}

func (r *inMemorySprintRepo) FindActive(_ context.Context, projectID string) (*sprintdomain.Sprint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.sprints {
		if s.ProjectID == projectID && s.State == sprintdomain.SprintStateActive {
			cp := s
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *inMemorySprintRepo) ListByProject(_ context.Context, projectID string) ([]sprintdomain.Sprint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []sprintdomain.Sprint
	for _, s := range r.sprints {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out, nil
}

// --- in-memory task repository ---

type inMemoryTaskRepo struct {
	mu      sync.RWMutex
	tasks   map[string]taskdomain.Task
	counter map[string]int
}

func newInMemoryTaskRepo() *inMemoryTaskRepo {
	return &inMemoryTaskRepo{
		tasks:   make(map[string]taskdomain.Task),
		counter: make(map[string]int),
	}
}

func (r *inMemoryTaskRepo) Save(_ context.Context, t taskdomain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
	return nil
}

func (r *inMemoryTaskRepo) FindByID(_ context.Context, id string) (taskdomain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return taskdomain.Task{}, errors.New("task not found")
	}
	return t, nil
}

func (r *inMemoryTaskRepo) FindByNumber(_ context.Context, projectID string, number int) (taskdomain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tasks {
		if t.ProjectID == projectID && t.Number == number {
			return t, nil
		}
	}
	return taskdomain.Task{}, errors.New("task not found")
}

func (r *inMemoryTaskRepo) List(_ context.Context, f taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []taskdomain.Task
	for _, t := range r.tasks {
		if !f.IncludeDeleted && t.DeletedAt != nil {
			continue
		}
		if f.ProjectID != "" && t.ProjectID != f.ProjectID {
			continue
		}
		if f.ColumnID != "" && t.ColumnID != f.ColumnID {
			continue
		}
		if f.AssigneeID != "" && (t.AssigneeID == nil || *t.AssigneeID != f.AssigneeID) {
			continue
		}
		if f.SprintID != "" && (t.SprintID == nil || *t.SprintID != f.SprintID) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *inMemoryTaskRepo) SoftDelete(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	t.DeletedAt = &at
	r.tasks[id] = t
	return nil
}

func (r *inMemoryTaskRepo) Restore(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	t.DeletedAt = nil
	r.tasks[id] = t
	return nil
}

func (r *inMemoryTaskRepo) NextNumber(_ context.Context, projectID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter[projectID]++
	return r.counter[projectID], nil
}
