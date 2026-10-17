package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
	"time"

	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
	widgetinfra "github.com/lesquel/termino/internal/features/widget/infrastructure"
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

func TestWidgetRepository_GetSummaryEmptyProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Seed a project and a column (required for widget queries).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO projects (id, key, name, methodology, created_at)
		VALUES ('wp1', 'WDG', 'Widget Project', 'kanban', ?)`, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO columns (id, project_id, name, position, wip_limit)
		VALUES ('wc1', 'wp1', 'In Progress', 1, 0)`); err != nil {
		t.Fatalf("seed column: %v", err)
	}

	repo := widgetinfra.NewWidgetRepository(db)
	summary, err := repo.GetSummary(ctx, "wp1", "cli")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.ProjectName != "Widget Project" {
		t.Errorf("ProjectName: got %q, want Widget Project", summary.ProjectName)
	}
	if summary.ProjectKey != "WDG" {
		t.Errorf("ProjectKey: got %q, want WDG", summary.ProjectKey)
	}
	// No tasks seeded — all slices should be empty.
	if len(summary.AssignedTasks) != 0 {
		t.Errorf("AssignedTasks: got %d, want 0", len(summary.AssignedTasks))
	}
}

// seedKanban creates project KAN with Backlog / In Progress / Done columns and
// a member "ana"; it returns a helper that inserts a task.
func seedKanban(t *testing.T, db *sql.DB) func(n int, title, col string, pos float64, assignee, due string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	stmts := []string{
		`INSERT INTO projects (id, key, name, methodology, created_at) VALUES ('kp', 'KAN', 'Kanban', 'kanban', '` + now + `')`,
		`INSERT INTO columns (id, project_id, name, position) VALUES ('backlog', 'kp', 'Backlog', 0)`,
		`INSERT INTO columns (id, project_id, name, position) VALUES ('doing', 'kp', 'In Progress', 1)`,
		`INSERT INTO columns (id, project_id, name, position) VALUES ('done', 'kp', 'Done', 2)`,
		`INSERT INTO members (id, name, kind) VALUES ('m-ana', 'ana', 'human')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	return func(n int, title, col string, pos float64, assignee, due string) {
		t.Helper()
		var a, d any
		if assignee != "" {
			a = assignee
		}
		if due != "" {
			d = due
		}
		if _, err := db.Exec(`
			INSERT INTO tasks (id, project_id, number, title, column_id, position, assignee_id, due_date, created_at, updated_at)
			VALUES (?, 'kp', ?, ?, ?, ?, ?, ?, ?, ?)`,
			title, n, title, col, pos, a, d, now, now); err != nil {
			t.Fatalf("seed task %q: %v", title, err)
		}
	}
}

func refs(ts []widgetdomain.TaskSummary) []string {
	out := make([]string, len(ts))
	for i, ts := range ts {
		out[i] = ts.Ref
	}
	return out
}

// A project whose only open work sits in Backlog must surface it: column
// counts plus the next Backlog tasks in board order.
func TestWidgetRepository_BacklogOnlyShowsNextTasks(t *testing.T) {
	db := openTestDB(t)
	add := seedKanban(t, db)
	add(1, "first", "backlog", 2, "", "")
	add(2, "second", "backlog", 1, "", "")
	add(3, "shipped", "done", 0, "", "")

	s, err := widgetinfra.NewWidgetRepository(db).GetSummary(context.Background(), "kp", "cli")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	wantCols := []widgetdomain.ColumnCount{{Name: "Backlog", Count: 2}, {Name: "In Progress", Count: 0}, {Name: "Done", Count: 1}}
	if !slices.Equal(s.Columns, wantCols) {
		t.Errorf("Columns = %v, want %v", s.Columns, wantCols)
	}
	if got := refs(s.NextTasks); !slices.Equal(got, []string{"KAN-2", "KAN-1"}) {
		t.Errorf("NextTasks = %v, want [KAN-2 KAN-1] (board order)", got)
	}
	if len(s.InProgressTasks) != 0 || len(s.AssignedTasks) != 0 {
		t.Errorf("unexpected active tasks: %+v / %+v", s.InProgressTasks, s.AssignedTasks)
	}
}

// Assigned and due-soon lists skip the Done column; the actor may be given by
// member name; overdue open tasks count as due soon.
func TestWidgetRepository_ActiveListsSkipDone(t *testing.T) {
	db := openTestDB(t)
	add := seedKanban(t, db)
	overdue := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	add(1, "mine open", "backlog", 0, "m-ana", overdue)
	add(2, "mine done", "done", 0, "m-ana", overdue)
	add(3, "working", "doing", 0, "", "")

	s, err := widgetinfra.NewWidgetRepository(db).GetSummary(context.Background(), "kp", "ana")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if got := refs(s.AssignedTasks); !slices.Equal(got, []string{"KAN-1"}) {
		t.Errorf("AssignedTasks = %v, want [KAN-1]", got)
	}
	if got := refs(s.InProgressTasks); !slices.Equal(got, []string{"KAN-3"}) {
		t.Errorf("InProgressTasks = %v, want [KAN-3]", got)
	}
	if got := refs(s.DueSoonTasks); !slices.Equal(got, []string{"KAN-1"}) {
		t.Errorf("DueSoonTasks = %v, want [KAN-1]", got)
	}
	if len(s.NextTasks) != 0 {
		t.Errorf("NextTasks = %v, want none while work is active", refs(s.NextTasks))
	}
}
