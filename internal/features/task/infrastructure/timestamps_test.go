package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
)

func TestTaskRepository_SaveStampsTimestampsAndKeepsUpdatedBy(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-ts", "TSA", "col-ts")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	task := taskdomain.Task{ID: "t-ts", ProjectID: "proj-ts", Number: 1, Title: "A",
		ColumnID: "col-ts", Priority: taskdomain.PriorityMedium, UpdatedBy: "Ana"}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, "t-ts")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.CreatedAt.Before(before) || got.UpdatedAt.Before(before) {
		t.Errorf("timestamps not stamped: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
	if got.UpdatedBy != "Ana" {
		t.Errorf("UpdatedBy = %q, want Ana", got.UpdatedBy)
	}
	list, err := repo.List(ctx, taskdomain.TaskFilter{ProjectID: "proj-ts"})
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v %v", list, err)
	}
	if list[0].UpdatedBy != "Ana" || list[0].CreatedAt.IsZero() {
		t.Errorf("List did not read timestamps: %+v", list[0])
	}
}

func TestTaskRepository_SaveImportedKeepsGivenValues(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-imp", "IMP", "col-imp")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	created := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 10, 17, 18, 10, 0, 0, time.UTC)
	task := taskdomain.Task{ID: "t-imp", ProjectID: "proj-imp", Number: 3, Title: "Imported",
		ColumnID: "col-imp", Priority: taskdomain.PriorityHigh,
		CreatedAt: created, UpdatedAt: updated, UpdatedBy: "Bob"}
	if err := repo.SaveImported(ctx, task); err != nil {
		t.Fatalf("SaveImported: %v", err)
	}
	task.Number = 4
	task.Title = "Imported again"
	if err := repo.SaveImported(ctx, task); err != nil {
		t.Fatalf("SaveImported update: %v", err)
	}
	got, err := repo.FindByID(ctx, "t-imp")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(updated) || got.UpdatedBy != "Bob" {
		t.Errorf("timestamps not kept: %+v", got)
	}
	if got.Number != 4 || got.Title != "Imported again" {
		t.Errorf("update not applied: number=%d title=%q", got.Number, got.Title)
	}
}

func TestTaskRepository_RenumberKeepsUpdatedAt(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-ren", "REN", "col-ren")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	task := taskdomain.Task{ID: "t-ren", ProjectID: "proj-ren", Number: 3, Title: "R",
		ColumnID: "col-ren", Priority: taskdomain.PriorityLow, CreatedAt: updated, UpdatedAt: updated}
	if err := repo.SaveImported(ctx, task); err != nil {
		t.Fatalf("SaveImported: %v", err)
	}
	if err := repo.Renumber(ctx, "t-ren", 7); err != nil {
		t.Fatalf("Renumber: %v", err)
	}
	got, err := repo.FindByID(ctx, "t-ren")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Number != 7 {
		t.Errorf("Number = %d, want 7", got.Number)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("Renumber changed updated_at to %v", got.UpdatedAt)
	}
}

func TestTaskRepository_SaveDuplicateNumber(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-dup", "DUP", "col-dup")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	a := taskdomain.Task{ID: "t-a", ProjectID: "proj-dup", Number: 1, Title: "A",
		ColumnID: "col-dup", Priority: taskdomain.PriorityMedium}
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	b := a
	b.ID = "t-b"
	err := repo.Save(ctx, b)
	if !errors.Is(err, taskdomain.ErrDuplicateNumber) {
		t.Fatalf("Save duplicate = %v, want ErrDuplicateNumber", err)
	}
}

func TestLabelRepository_ListByProject(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-lbl", "LBL", "col-lbl")
	repo := taskinfra.NewLabelRepository(db)
	ctx := context.Background()
	for _, l := range []taskdomain.Label{{ID: "l2", ProjectID: "proj-lbl", Name: "ui"}, {ID: "l1", ProjectID: "proj-lbl", Name: "bug", Color: "#f00"}} {
		if err := repo.Save(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListByProject(ctx, "proj-lbl")
	if err != nil || len(got) != 2 || got[0].Name != "bug" || got[0].Color != "#f00" || got[1].Name != "ui" {
		t.Errorf("ListByProject = %+v, %v", got, err)
	}
}
