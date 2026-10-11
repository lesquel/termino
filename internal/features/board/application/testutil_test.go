package application_test

import (
	"context"
	"errors"
	"sync"
	"time"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// --- in-memory board repository ---

type inMemoryBoardRepo struct {
	mu      sync.RWMutex
	columns map[string]boarddomain.Column
}

func newInMemoryBoardRepo() *inMemoryBoardRepo {
	return &inMemoryBoardRepo{columns: make(map[string]boarddomain.Column)}
}

func (r *inMemoryBoardRepo) SaveColumn(_ context.Context, col boarddomain.Column) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.columns[col.ID] = col
	return nil
}

func (r *inMemoryBoardRepo) FindColumnByID(_ context.Context, id string) (boarddomain.Column, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	col, ok := r.columns[id]
	if !ok {
		return boarddomain.Column{}, boarddomain.ErrColumnNotFound
	}
	return col, nil
}

func (r *inMemoryBoardRepo) ListColumns(_ context.Context, projectID string) ([]boarddomain.Column, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []boarddomain.Column
	for _, col := range r.columns {
		if col.ProjectID == projectID {
			out = append(out, col)
		}
	}
	return out, nil
}

func (r *inMemoryBoardRepo) ReorderColumns(_ context.Context, _ string, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for pos, id := range ids {
		col, ok := r.columns[id]
		if !ok {
			continue
		}
		col.Position = pos
		r.columns[id] = col
	}
	return nil
}

func (r *inMemoryBoardRepo) DeleteColumn(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.columns, id)
	return nil
}

// --- in-memory task repository ---

type inMemoryTaskRepo struct {
	mu      sync.RWMutex
	tasks   map[string]taskdomain.Task
	counter map[string]int // project -> last number
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
