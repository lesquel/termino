package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedProject inserts a minimal project and column so FK constraints pass.
func seedProjectAndColumn(t *testing.T, db *sql.DB, projID, projKey, colID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO projects (id, key, name, methodology, created_at)
		VALUES (?, ?, ?, 'kanban', ?)`,
		projID, projKey, projKey+" Project", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seedProject: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO columns (id, project_id, name, position, wip_limit)
		VALUES (?, ?, 'Backlog', 0, 0)`, colID, projID)
	if err != nil {
		t.Fatalf("seedColumn: %v", err)
	}
}

func TestTaskRepository_SaveAndFindByID(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-task", "TSK", "col-task")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	task := taskdomain.Task{
		ID:        "task-1",
		ProjectID: "proj-task",
		Number:    1,
		Title:     "First task",
		ColumnID:  "col-task",
		Priority:  taskdomain.PriorityMedium,
		Position:  1.0,
	}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByID(ctx, "task-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Title != task.Title {
		t.Errorf("Title: got %q, want %q", got.Title, task.Title)
	}
	if got.Priority != task.Priority {
		t.Errorf("Priority: got %q, want %q", got.Priority, task.Priority)
	}
}

func TestTaskRepository_FindByNumber(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-num", "NUM", "col-num")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	task := taskdomain.Task{
		ID:        "task-num-1",
		ProjectID: "proj-num",
		Number:    42,
		Title:     "Numbered task",
		ColumnID:  "col-num",
		Priority:  taskdomain.PriorityHigh,
		Position:  1.0,
	}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.FindByNumber(ctx, "proj-num", 42)
	if err != nil {
		t.Fatalf("FindByNumber: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("ID: got %q, want %q", got.ID, task.ID)
	}
}

func TestTaskRepository_ListWithFilter(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-list", "LTF", "col-ltf")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	tasks := []taskdomain.Task{
		{ID: "lt1", ProjectID: "proj-list", Number: 1, Title: "Alpha task", ColumnID: "col-ltf", Priority: taskdomain.PriorityLow, Position: 1},
		{ID: "lt2", ProjectID: "proj-list", Number: 2, Title: "Beta task", ColumnID: "col-ltf", Priority: taskdomain.PriorityMedium, Position: 2},
	}
	for _, tk := range tasks {
		if err := repo.Save(ctx, tk); err != nil {
			t.Fatalf("Save %s: %v", tk.ID, err)
		}
	}

	all, err := repo.List(ctx, taskdomain.TaskFilter{ProjectID: "proj-list"})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List all: got %d, want 2", len(all))
	}

	// Filter by search text.
	filtered, err := repo.List(ctx, taskdomain.TaskFilter{ProjectID: "proj-list", SearchText: "Alpha"})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("List filtered: got %d, want 1", len(filtered))
	}
}

func TestTaskRepository_SoftDeleteAndRestore(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-del", "DEL", "col-del")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	task := taskdomain.Task{
		ID:        "del-task",
		ProjectID: "proj-del",
		Number:    1,
		Title:     "Delete me",
		ColumnID:  "col-del",
		Priority:  taskdomain.PriorityLow,
		Position:  1,
	}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Soft delete.
	if err := repo.SoftDelete(ctx, "del-task", time.Now().UTC()); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// List without IncludeDeleted should not include it.
	list, err := repo.List(ctx, taskdomain.TaskFilter{ProjectID: "proj-del"})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 tasks after soft delete, got %d", len(list))
	}

	// List with IncludeDeleted should include it.
	listAll, err := repo.List(ctx, taskdomain.TaskFilter{ProjectID: "proj-del", IncludeDeleted: true})
	if err != nil {
		t.Fatalf("List with deleted: %v", err)
	}
	if len(listAll) != 1 {
		t.Errorf("expected 1 task with IncludeDeleted, got %d", len(listAll))
	}

	// Restore.
	if err := repo.Restore(ctx, "del-task"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	listRestored, err := repo.List(ctx, taskdomain.TaskFilter{ProjectID: "proj-del"})
	if err != nil {
		t.Fatalf("List after restore: %v", err)
	}
	if len(listRestored) != 1 {
		t.Errorf("expected 1 task after restore, got %d", len(listRestored))
	}
}

func TestTaskRepository_NextNumber(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-nn", "NNN", "col-nn")
	repo := taskinfra.NewTaskRepository(db)
	ctx := context.Background()

	n1, err := repo.NextNumber(ctx, "proj-nn")
	if err != nil {
		t.Fatalf("NextNumber (empty): %v", err)
	}
	if n1 != 1 {
		t.Errorf("NextNumber (empty): got %d, want 1", n1)
	}

	// Save one task.
	task := taskdomain.Task{
		ID:        "nn-task",
		ProjectID: "proj-nn",
		Number:    1,
		Title:     "First",
		ColumnID:  "col-nn",
		Priority:  taskdomain.PriorityMedium,
		Position:  1,
	}
	if err := repo.Save(ctx, task); err != nil {
		t.Fatalf("Save: %v", err)
	}

	n2, err := repo.NextNumber(ctx, "proj-nn")
	if err != nil {
		t.Fatalf("NextNumber (after one): %v", err)
	}
	if n2 != 2 {
		t.Errorf("NextNumber (after one): got %d, want 2", n2)
	}
}

func TestCommentRepository_SaveAndList(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-cmt", "CMT", "col-cmt")
	// Seed a member for FK.
	_, err := db.Exec(`INSERT INTO members (id, name, kind) VALUES ('m1', 'Alice', 'human')`)
	if err != nil {
		t.Fatalf("seedMember: %v", err)
	}
	taskRepo := taskinfra.NewTaskRepository(db)
	commentRepo := taskinfra.NewCommentRepository(db)
	ctx := context.Background()

	task := taskdomain.Task{
		ID:        "cmt-task",
		ProjectID: "proj-cmt",
		Number:    1,
		Title:     "Commented task",
		ColumnID:  "col-cmt",
		Priority:  taskdomain.PriorityMedium,
		Position:  1,
	}
	if err := taskRepo.Save(ctx, task); err != nil {
		t.Fatalf("Save task: %v", err)
	}

	comment := taskdomain.Comment{
		ID:        "c1",
		TaskID:    "cmt-task",
		AuthorID:  "m1",
		Body:      "Hello world",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := commentRepo.Save(ctx, comment); err != nil {
		t.Fatalf("Save comment: %v", err)
	}

	list, err := commentRepo.ListByTask(ctx, "cmt-task")
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByTask: got %d, want 1", len(list))
	}
	if list[0].Body != comment.Body {
		t.Errorf("Body: got %q, want %q", list[0].Body, comment.Body)
	}
}

func TestActivityRepository_AppendAndList(t *testing.T) {
	db := openTestDB(t)
	seedProjectAndColumn(t, db, "proj-act", "ACT", "col-act")
	// Seed a member so the actor FK passes.
	_, err := db.Exec(`INSERT INTO members (id, name, kind) VALUES ('m1', 'Actor', 'human')`)
	if err != nil {
		t.Fatalf("seedMember: %v", err)
	}
	taskRepo := taskinfra.NewTaskRepository(db)
	actRepo := taskinfra.NewActivityRepository(db)
	ctx := context.Background()

	task := taskdomain.Task{
		ID:        "act-task",
		ProjectID: "proj-act",
		Number:    1,
		Title:     "Active task",
		ColumnID:  "col-act",
		Priority:  taskdomain.PriorityMedium,
		Position:  1,
	}
	if err := taskRepo.Save(ctx, task); err != nil {
		t.Fatalf("Save task: %v", err)
	}

	activity := taskdomain.Activity{
		ID:        "a1",
		TaskID:    "act-task",
		ActorID:   "m1",
		Action:    "created",
		Payload:   map[string]any{"title": "Active task"},
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := actRepo.Append(ctx, activity); err != nil {
		t.Fatalf("Append: %v", err)
	}

	list, err := actRepo.ListByTask(ctx, "act-task")
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByTask: got %d, want 1", len(list))
	}
	if list[0].Action != "created" {
		t.Errorf("Action: got %q, want created", list[0].Action)
	}

	recent, err := actRepo.ListRecent(ctx, "proj-act", 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("ListRecent: got %d, want 1", len(recent))
	}
}
