package application_test

import (
	"context"
	"errors"
	"sync"
	"time"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
)

// --- in-memory project repository ---

type inMemoryProjectRepo struct {
	mu    sync.RWMutex
	byID  map[string]projectdomain.Project
	byKey map[string]projectdomain.Project
}

func newInMemoryProjectRepo() *inMemoryProjectRepo {
	return &inMemoryProjectRepo{
		byID:  make(map[string]projectdomain.Project),
		byKey: make(map[string]projectdomain.Project),
	}
}

func (r *inMemoryProjectRepo) Save(_ context.Context, p projectdomain.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[p.ID] = p
	r.byKey[p.Key] = p
	return nil
}

func (r *inMemoryProjectRepo) FindByID(_ context.Context, id string) (projectdomain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byID[id]
	if !ok {
		return projectdomain.Project{}, errors.New("project not found")
	}
	return p, nil
}

func (r *inMemoryProjectRepo) FindByKey(_ context.Context, key string) (projectdomain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byKey[key]
	if !ok {
		return projectdomain.Project{}, errors.New("project not found")
	}
	return p, nil
}

func (r *inMemoryProjectRepo) List(_ context.Context) ([]projectdomain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]projectdomain.Project, 0, len(r.byID))
	for _, p := range r.byID {
		if p.ArchivedAt == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *inMemoryProjectRepo) Archive(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byID[id]
	if !ok {
		return errors.New("project not found")
	}
	p.ArchivedAt = &at
	r.byID[id] = p
	r.byKey[p.Key] = p
	return nil
}

// --- in-memory board repository ---

type inMemoryBoardRepo struct {
	mu      sync.RWMutex
	columns map[string]boarddomain.Column // keyed by column ID
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

func (r *inMemoryBoardRepo) ReorderColumns(_ context.Context, projectID string, ids []string) error {
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
