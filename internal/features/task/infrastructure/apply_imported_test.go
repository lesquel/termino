package infrastructure_test

import (
	"context"
	"testing"
	"time"

	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
)

// TestTaskRepository_ApplyImportedSwapsNumbers: an import may hand a task the
// number another one still holds (a renumber on another machine, or a
// duplicate-number loser); the whole set lands in one transaction.
func TestTaskRepository_ApplyImportedSwapsNumbers(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-sw", "SWP", "col-sw")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()
	at := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	mk := func(id string, n int) taskdomain.Task {
		return taskdomain.Task{ID: id, ProjectID: "proj-sw", Number: n, Title: id, ColumnID: "col-sw",
			Priority: taskdomain.PriorityMedium, CreatedAt: at, UpdatedAt: at}
	}
	for _, task := range []taskdomain.Task{mk("x", 1), mk("y", 2)} {
		if err := repo.SaveImported(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	// y takes 1 from its file, x (unchanged locally) is renumbered to 3, and a
	// new task z claims 2.
	if err := repo.ApplyImported(ctx, map[string]int{"x": 3}, []taskdomain.Task{mk("y", 1), mk("z", 2)}); err != nil {
		t.Fatalf("ApplyImported: %v", err)
	}
	for id, want := range map[string]int{"x": 3, "y": 1, "z": 2} {
		got, err := repo.FindByID(ctx, id)
		if err != nil || got.Number != want {
			t.Errorf("%s number = %d (%v), want %d", id, got.Number, err, want)
		}
	}
	x, _ := repo.FindByID(ctx, "x")
	if !x.UpdatedAt.Equal(at) {
		t.Errorf("renumber touched updated_at: %v", x.UpdatedAt)
	}
}
