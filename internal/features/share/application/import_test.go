package application_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	shareapp "github.com/lesquel/termino/internal/features/share/application"
	sharefile "github.com/lesquel/termino/internal/features/share/domain"
	sprintinfra "github.com/lesquel/termino/internal/features/sprint/infrastructure"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
)

// pull makes dst's .termino an exact copy of src's, like a git pull.
func pull(t *testing.T, src, dst *fixture) {
	t.Helper()
	dir := filepath.Join(dst.root, sharefile.Dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	for rel, content := range src.snapshot() {
		writeShared(t, dst, rel, content)
	}
}

func writeShared(t *testing.T, f *fixture, rel, content string) {
	t.Helper()
	path := filepath.Join(f.root, sharefile.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) mustImport() shareapp.ImportResult {
	f.t.Helper()
	res, err := f.svc.Import(f.ctx, f.project.ID)
	if err != nil {
		f.t.Fatalf("Import: %v", err)
	}
	return res
}

func (f *fixture) task(id string) taskdomain.Task {
	f.t.Helper()
	task, err := taskinfra.NewTaskRepository(f.db).FindByID(f.ctx, id)
	if err != nil {
		f.t.Fatalf("FindByID(%s): %v", id, err)
	}
	return task
}

// numbers maps task titles to numbers for the live tasks of the project.
func (f *fixture) numbers() map[string]int {
	f.t.Helper()
	tasks, err := taskinfra.NewTaskRepository(f.db).List(f.ctx, taskdomain.TaskFilter{ProjectID: f.project.ID})
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]int{}
	for _, task := range tasks {
		out[task.Title] = task.Number
	}
	return out
}

func TestImport_NewUpdatedAndDeletedTasks(t *testing.T) {
	a, b := newFixture(t, true), newFixture(t, true)
	created := a.createTask("Write docs")
	pull(t, a, b)
	if res := b.mustImport(); res.Changed == 0 || len(res.Skipped) != 0 {
		t.Fatalf("import new = %+v", res)
	}
	got := b.task(created.ID)
	want := a.task(created.ID)
	if got.Title != "Write docs" || got.Number != want.Number {
		t.Errorf("imported task = %+v", got)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) || got.UpdatedBy != want.UpdatedBy {
		t.Errorf("timestamps not preserved: got %v/%v/%q want %v/%v/%q",
			got.CreatedAt, got.UpdatedAt, got.UpdatedBy, want.CreatedAt, want.UpdatedAt, want.UpdatedBy)
	}

	if err := a.tasks.MoveTask(a.ctx, actor, created.ID, a.column("Done"), 1); err != nil {
		t.Fatal(err)
	}
	pull(t, a, b)
	b.mustImport()
	if got := b.task(created.ID); got.ColumnID != b.column("Done") {
		t.Errorf("moved task column = %s, want Done %s", got.ColumnID, b.column("Done"))
	}

	if err := a.tasks.DeleteTask(a.ctx, actor, created.ID, true); err != nil {
		t.Fatal(err)
	}
	pull(t, a, b)
	b.mustImport()
	if got := b.task(created.ID); got.DeletedAt == nil {
		t.Error("task whose file vanished is not soft-deleted")
	}
}

func TestImport_IsIdempotentAndNeverExports(t *testing.T) {
	a, b := newFixture(t, true), newFixture(t, true)
	a.createTask("One")
	a.createTask("Two")
	pull(t, a, b)
	before := b.snapshot()
	writes := b.files.writes
	b.mustImport()
	if b.files.writes != writes {
		t.Errorf("import wrote %d files, want none", b.files.writes-writes)
	}
	if diff := changed(before, b.snapshot()); len(diff) != 0 {
		t.Errorf("import changed shared files %v", diff)
	}
	if res := b.mustImport(); res.Changed != 0 {
		t.Errorf("second import changed %d, want 0", res.Changed)
	}
	if b.logs.Len() != 0 {
		t.Errorf("unexpected log output: %s", b.logs.String())
	}
}

func TestImport_CreatesUnknownEntities(t *testing.T) {
	f := newFixture(t, true)
	id := "11111111-2222-3333-4444-555555555555"
	writeShared(t, f, sharefile.TaskFilePath(id), `version = 1
id = "`+id+`"
number = 7
title = "From a teammate"
column = "QA"
position = 1
priority = "high"
assignee = "Zoe"
labels = ["infra"]
sprint = "Sprint 9"
created_at = 2026-10-17T18:00:00Z
updated_at = 2026-10-17T18:10:00Z
updated_by = "Zoe"
`)
	if res := f.mustImport(); len(res.Skipped) != 0 {
		t.Fatalf("skipped %v", res.Skipped)
	}
	got := f.task(id)
	if got.Number != 7 || got.Priority != taskdomain.PriorityHigh {
		t.Errorf("task = %+v", got)
	}
	if got.CreatedAt != time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC) || got.UpdatedBy != "Zoe" {
		t.Errorf("timestamps = %v %q", got.CreatedAt, got.UpdatedBy)
	}
	cols, _ := f.board.GetBoard(f.ctx, f.project.ID)
	last := cols.Columns[0]
	for _, c := range cols.Columns {
		if c.Position > last.Position {
			last = c
		}
	}
	if last.Name != "QA" || got.ColumnID != last.ID {
		t.Errorf("unknown column not created at the end: last=%+v task column=%s", last, got.ColumnID)
	}
	zoe, err := memberinfra.NewMemberRepository(f.db).FindByName(f.ctx, "Zoe")
	if err != nil || zoe.Kind != "human" || got.AssigneeID == nil || *got.AssigneeID != zoe.ID {
		t.Errorf("member Zoe = %+v, %v; assignee %v", zoe, err, got.AssigneeID)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "infra" {
		t.Errorf("labels = %v", got.Labels)
	}
	sprints, _ := sprintinfra.NewSprintRepository(f.db).ListByProject(f.ctx, f.project.ID)
	if len(sprints) != 1 || sprints[0].Name != "Sprint 9" || sprints[0].State != "draft" ||
		got.SprintID == nil || *got.SprintID != sprints[0].ID {
		t.Errorf("sprints = %+v, task sprint %v", sprints, got.SprintID)
	}
}

func TestImport_SkipsBrokenFilesWithReasons(t *testing.T) {
	f := newFixture(t, true)
	good := f.createTask("Fine")
	conflict := "aaaaaaaa-0000-0000-0000-000000000001"
	invalid := "aaaaaaaa-0000-0000-0000-000000000002"
	newer := "aaaaaaaa-0000-0000-0000-000000000003"
	writeShared(t, f, sharefile.TaskFilePath(conflict), "version = 1\n<<<<<<< HEAD\ntitle = \"a\"\n=======\ntitle = \"b\"\n>>>>>>> other\n")
	writeShared(t, f, sharefile.TaskFilePath(invalid), "version = 1\ntitle = \n")
	writeShared(t, f, sharefile.TaskFilePath(newer), "version = 9\nid = \""+newer+"\"\ntitle = \"x\"\n")
	writeShared(t, f, "tasks/.scratch.toml", "garbage")
	writeShared(t, f, "tasks/.x.toml.tmp", "garbage")

	res := f.mustImport()
	reasons := map[string]string{}
	for _, s := range res.Skipped {
		reasons[s.Path] = s.Reason
	}
	if len(reasons) != 3 {
		t.Fatalf("skipped = %+v, want the 3 broken files", res.Skipped)
	}
	for path, want := range map[string]string{
		sharefile.TaskFilePath(conflict): "conflict markers",
		sharefile.TaskFilePath(invalid):  "invalid",
		sharefile.TaskFilePath(newer):    "version",
	} {
		if !strings.Contains(reasons[path], want) {
			t.Errorf("reason for %s = %q, want it to mention %q", path, reasons[path], want)
		}
	}
	if f.task(good.ID).DeletedAt != nil {
		t.Error("a valid task was affected by broken files")
	}
	st, err := f.svc.Status(f.ctx, f.project.ID)
	if err != nil || st.At.IsZero() || len(st.Skipped) != 3 {
		t.Errorf("Status = %+v, %v", st, err)
	}

	// Resolving the conflict lets the next import pick the file up.
	writeShared(t, f, sharefile.TaskFilePath(conflict), "version = 1\nid = \""+conflict+"\"\nnumber = 40\ntitle = \"Resolved\"\ncolumn = \"Backlog\"\nposition = 1\npriority = \"low\"\ncreated_at = 2026-10-17T18:00:00Z\nupdated_at = 2026-10-17T18:00:00Z\n")
	res = f.mustImport()
	if len(res.Skipped) != 2 || f.task(conflict).Title != "Resolved" {
		t.Errorf("after resolving: skipped %v", res.Skipped)
	}
}

func TestImport_MissingProjectFileImportsNothing(t *testing.T) {
	f := newFixture(t, true)
	task := f.createTask("Keep me")
	if err := os.RemoveAll(filepath.Join(f.root, sharefile.Dir)); err != nil {
		t.Fatal(err)
	}
	res := f.mustImport()
	if len(res.Skipped) != 1 || res.Skipped[0].Path != sharefile.ProjectFileName {
		t.Errorf("skipped = %v", res.Skipped)
	}
	if f.task(task.ID).DeletedAt != nil {
		t.Error("a missing .termino directory deleted tasks")
	}
}

// TestImport_DuplicateNumbersConverge simulates two machines creating a task
// concurrently (both get number 1) and merging: every machine, whatever it
// had locally, ends with the same numbering and byte-identical files.
func TestImport_DuplicateNumbersConverge(t *testing.T) {
	a, b, c := newFixture(t, true), newFixture(t, true), newFixture(t, true)
	a.createTask("Base")
	pull(t, a, b)
	b.mustImport()
	pull(t, a, c)
	c.mustImport()

	fromA := a.createTask("From A")
	time.Sleep(1100 * time.Millisecond) // created_at has second precision
	fromB := b.createTask("From B")
	if fromA.Number != fromB.Number {
		t.Fatalf("setup: numbers %d and %d should collide", fromA.Number, fromB.Number)
	}

	// Merge: each machine receives the other's new file; c receives both.
	merged := a.snapshot()
	for rel, content := range b.snapshot() {
		if strings.HasPrefix(rel, sharefile.TasksDir+"/") {
			merged[rel] = content
		}
	}
	for _, f := range []*fixture{a, b, c} {
		for rel, content := range merged {
			writeShared(t, f, rel, content)
		}
		f.mustImport()
	}

	want := map[string]int{"Base": 1, "From A": 2, "From B": 3}
	for name, f := range map[string]*fixture{"a": a, "b": b, "c": c} {
		if got := f.numbers(); !equalNumbers(got, want) {
			t.Errorf("machine %s numbers = %v, want %v", name, got, want)
		}
	}
	files := func(f *fixture) []string {
		var out []string
		for rel, content := range f.snapshot() {
			if strings.HasPrefix(rel, sharefile.TasksDir+"/") {
				out = append(out, rel+"\n"+content)
			}
		}
		sort.Strings(out)
		return out
	}
	if fa, fb, fc := files(a), files(b), files(c); strings.Join(fa, "") != strings.Join(fb, "") || strings.Join(fa, "") != strings.Join(fc, "") {
		t.Errorf("task files differ between machines:\nA %v\nB %v\nC %v", fa, fb, fc)
	}
	if got := a.taskFile(fromB.ID).Number; got != 3 {
		t.Errorf("loser file number = %d, want 3", got)
	}
}

func TestImport_NotSharedIsNoop(t *testing.T) {
	f := newFixture(t, false)
	res, err := f.svc.Import(f.ctx, f.project.ID)
	if err != nil || res.Changed != 0 {
		t.Errorf("Import = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(f.root, sharefile.Dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unshared import touched files: %v", err)
	}
}

func equalNumbers(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
