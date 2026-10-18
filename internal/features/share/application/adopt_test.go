package application_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	shareapp "github.com/lesquel/termino/internal/features/share/application"
)

// fresh returns a fixture whose database has no project with the key of
// a's shared board ("PROJ"), and whose share root holds a copy of a's files.
func freshClone(t *testing.T, a *fixture) *fixture {
	t.Helper()
	b := newFixture(t, false)
	if err := projectinfra.NewProjectRepository(b.db).Archive(b.ctx, b.project.ID, b.project.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := b.db.Exec(`UPDATE projects SET key = 'OLD' WHERE id = ?`, b.project.ID); err != nil {
		t.Fatal(err)
	}
	pull(t, a, b)
	return b
}

func TestAdopt_CreatesProjectFromClonedRepo(t *testing.T) {
	a := newFixture(t, true)
	task := a.createTask("Shared work")
	b := freshClone(t, a)

	res, err := b.svc.Adopt(b.ctx, b.root)
	if err != nil || res.Action != shareapp.AdoptCreated {
		t.Fatalf("Adopt = %+v, %v", res, err)
	}
	p, err := projectinfra.NewProjectRepository(b.db).FindByID(b.ctx, res.ProjectID)
	if err != nil || p.Key != "PROJ" || p.ShareRoot != filepath.Clean(b.root) || p.AssociatedPath != filepath.Clean(b.root) {
		t.Fatalf("project = %+v, %v", p, err)
	}
	b.project = p
	if got := b.task(task.ID); got.Title != "Shared work" {
		t.Errorf("task not imported: %+v", got)
	}
	if b.column("Done") == "" {
		t.Error("columns not created")
	}
	if res, err := b.svc.Adopt(b.ctx, b.root); err != nil || res.Action != shareapp.AdoptAlready {
		t.Errorf("second Adopt = %+v, %v", res, err)
	}
}

func TestAdopt_LinksExistingProjectAndMergesByUUID(t *testing.T) {
	a := newFixture(t, true)
	shared := a.createTask("Shared")
	b := newFixture(t, false) // same key PROJ, not shared yet
	local := b.createTask("Local only")
	pull(t, a, b)

	res, err := b.svc.Adopt(b.ctx, b.root)
	if err != nil || res.Action != shareapp.AdoptLinked || res.ProjectID != b.project.ID {
		t.Fatalf("Adopt = %+v, %v", res, err)
	}
	if b.task(shared.ID).Title != "Shared" {
		t.Error("shared task not imported")
	}
	// The local task is exported (renumbered: both claimed number 1).
	if tf := b.taskFile(local.ID); tf.Title != "Local only" || tf.Number == b.task(shared.ID).Number {
		t.Errorf("local task file = %+v", tf)
	}
}

func TestAdopt_KeySharedFromAnotherRootFails(t *testing.T) {
	a := newFixture(t, true)
	b := newFixture(t, true) // PROJ already shared from b.root
	other := t.TempDir()
	for rel, content := range a.snapshot() {
		writeShared(t, &fixture{t: t, root: other}, rel, content)
	}
	_, err := b.svc.Adopt(b.ctx, other)
	if !errors.Is(err, shareapp.ErrSharedElsewhere) || !strings.Contains(err.Error(), b.root) || !strings.Contains(err.Error(), other) {
		t.Fatalf("Adopt error = %v", err)
	}
	p, _ := projectinfra.NewProjectRepository(b.db).FindByID(b.ctx, b.project.ID)
	if p.ShareRoot != b.root {
		t.Errorf("share root changed to %q", p.ShareRoot)
	}
}

func TestAdopt_NoSharedBoardIsNoop(t *testing.T) {
	b := newFixture(t, false)
	if res, err := b.svc.Adopt(b.ctx, t.TempDir()); err != nil || res.Action != shareapp.AdoptNone {
		t.Errorf("Adopt = %+v, %v", res, err)
	}
}

func TestOff_StopsSharing(t *testing.T) {
	f := newFixture(t, true)
	if err := f.svc.Off(f.ctx, f.project.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.mutate(func() { f.createTask("After off") }); len(got) != 0 {
		t.Errorf("export after off changed %v", got)
	}
	p, _ := projectinfra.NewProjectRepository(f.db).FindByID(f.ctx, f.project.ID)
	if p.ShareRoot != "" {
		t.Errorf("share root = %q", p.ShareRoot)
	}
	if len(f.snapshot()) == 0 {
		t.Error("off removed the shared files")
	}
}

func TestAdopt_DoesNotRelinkAfterOff(t *testing.T) {
	f := newFixture(t, true)
	if err := f.svc.Off(f.ctx, f.project.ID); err != nil {
		t.Fatal(err)
	}
	if res, err := f.svc.Adopt(f.ctx, f.root); err != nil || res.Action != shareapp.AdoptNone {
		t.Fatalf("Adopt after off = %+v, %v", res, err)
	}
	if err := f.svc.Init(f.ctx, f.project.ID, f.root); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Off(f.ctx, f.project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`DELETE FROM share_optouts`); err != nil {
		t.Fatal(err)
	}
	if res, err := f.svc.Adopt(f.ctx, f.root); err != nil || res.Action != shareapp.AdoptLinked {
		t.Errorf("Adopt without opt-out = %+v, %v", res, err)
	}
}
