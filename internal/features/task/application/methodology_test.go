package application_test

import (
	"context"
	"errors"
	"testing"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
)

var errNoSkipping = errors.New("cards cannot skip columns")

// strictMethodology rejects any move to a column named "Done".
type strictMethodology struct {
	projectdomain.KanbanMethodology
}

func (strictMethodology) CanMove(_ projectdomain.Task, to projectdomain.Column, _ []projectdomain.Column) error {
	if to.Name == "Done" {
		return errNoSkipping
	}
	return nil
}

type fixedMethodologies struct {
	m        projectdomain.Methodology
	projects []string
}

func (f *fixedMethodologies) MethodologyFor(_ context.Context, projectID string) (projectdomain.Methodology, error) {
	f.projects = append(f.projects, projectID)
	return f.m, nil
}

func TestMoveTask_EnforcesMethodologyCanMove(t *testing.T) {
	ctx := context.Background()
	boardRepo := newInMemoryBoardRepo()
	tasks := newInMemoryTaskRepo()
	_ = boardRepo.SaveColumn(ctx, boarddomain.Column{ID: "col-1", ProjectID: "proj-1", Name: "Backlog", Position: 0})
	_ = boardRepo.SaveColumn(ctx, boarddomain.Column{ID: "col-2", ProjectID: "proj-1", Name: "In Progress", Position: 1})
	_ = boardRepo.SaveColumn(ctx, boarddomain.Column{ID: "col-3", ProjectID: "proj-1", Name: "Done", Position: 2})
	resolver := &fixedMethodologies{m: strictMethodology{}}
	uc := newTaskUC(tasks, newInMemoryCommentRepo(), newInMemoryActivityRepo(), boardRepo).WithMethodologies(resolver)

	task, err := uc.CreateTask(ctx, testActor(), defaultInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := uc.MoveTask(ctx, testActor(), task.ID, "col-3", 1); !errors.Is(err, errNoSkipping) {
		t.Fatalf("move to Done err = %v, want %v", err, errNoSkipping)
	}
	got, _ := tasks.FindByID(ctx, task.ID)
	if got.ColumnID != "col-1" {
		t.Fatalf("rejected move changed column to %q", got.ColumnID)
	}
	if err := uc.MoveTask(ctx, testActor(), task.ID, "col-2", 1); err != nil {
		t.Fatalf("allowed move: %v", err)
	}
	if len(resolver.projects) == 0 || resolver.projects[0] != "proj-1" {
		t.Fatalf("resolver asked for %v, want proj-1", resolver.projects)
	}
}
