package application_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"database/sql"

	sharedomain "github.com/lesquel/termino/internal/domain"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	shareapp "github.com/lesquel/termino/internal/features/share/application"
	sharefile "github.com/lesquel/termino/internal/features/share/domain"
	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
	sprintapp "github.com/lesquel/termino/internal/features/sprint/application"
	sprintinfra "github.com/lesquel/termino/internal/features/sprint/infrastructure"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

var actor = sharedomain.Actor{ID: "ana", Name: "Ana", Kind: sharedomain.ActorKindHuman}

// fixture wires the use cases the way internal/cli/container.go does: every
// repository the use cases write through is wrapped by the share service.
type fixture struct {
	t        *testing.T
	ctx      context.Context
	db       *sql.DB
	root     string
	svc      *shareapp.ShareService
	logs     *bytes.Buffer
	projects *projectapp.ProjectUseCases
	board    *boardapp.BoardUseCases
	tasks    *taskapp.TaskUseCases
	members  *memberapp.MemberUseCases
	sprints  *sprintapp.SprintUseCases
	memberDB memberdomain.MemberRepository
	project  projectdomain.Project
	files    *countingStore
}

// countingStore counts the writes and removals that reach the filesystem.
type countingStore struct {
	shareinfra.FileStore
	writes int
}

func (c *countingStore) Write(path string, data []byte) error {
	c.writes++
	return c.FileStore.Write(path, data)
}

func (c *countingStore) Remove(path string) error {
	c.writes++
	return c.FileStore.Remove(path)
}

func newFixture(t *testing.T, shared bool) *fixture {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))

	labelRepo := taskinfra.NewLabelRepository(db)
	files := &countingStore{}
	svc := shareapp.NewShareService(shareapp.Deps{
		Projects: projectinfra.NewProjectRepository(db),
		Board:    boardinfra.NewBoardRepository(db),
		Tasks:    taskinfra.NewTaskRepository(db),
		Members:  memberinfra.NewMemberRepository(db),
		Labels:   labelRepo,
		Sprints:  sprintinfra.NewSprintRepository(db),
		Files:    files,
		Hashes:   shareinfra.NewHashRepository(db),
		Codec:    shareinfra.TOMLCodec{},
		State:    shareinfra.NewImportStateRepository(db),
		Logger:   func() *slog.Logger { return logger },
	})
	// The actor must exist: activity rows reference members(id).
	if err := memberinfra.NewMemberRepository(db).Save(context.Background(), memberdomain.Member{
		ID: actor.ID, Name: actor.Name, Kind: memberdomain.KindHuman, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: context.Background(), db: db, root: t.TempDir(), svc: svc, logs: logs, files: files}
	taskRepo := svc.Tasks(taskinfra.NewTaskRepository(db))
	boardRepo := svc.Board(boardinfra.NewBoardRepository(db))
	f.projects = projectapp.NewProjectUseCases(svc.Projects(projectinfra.NewProjectRepository(db)), boardRepo, events.NewBus())
	f.board = boardapp.NewBoardUseCases(boardRepo, taskRepo)
	f.tasks = taskapp.NewTaskUseCases(taskRepo, taskinfra.NewCommentRepository(db),
		taskinfra.NewActivityRepository(db), boardRepo, events.NewBus()).WithLabels(svc.Labels(labelRepo))
	f.memberDB = svc.Members(memberinfra.NewMemberRepository(db))
	f.members = memberapp.NewMemberUseCases(f.memberDB)
	f.sprints = sprintapp.NewSprintUseCases(svc.Sprints(sprintinfra.NewSprintRepository(db)), taskRepo)

	p, err := f.projects.CreateProject(f.ctx, actor, projectapp.CreateProjectInput{Key: "PROJ", Name: "termino", Methodology: projectdomain.MethodologyKanban})
	if err != nil {
		t.Fatal(err)
	}
	if shared {
		if err := svc.Init(f.ctx, p.ID, f.root); err != nil {
			t.Fatalf("Init: %v", err)
		}
		p, _ = projectinfra.NewProjectRepository(db).FindByID(f.ctx, p.ID)
	}
	f.project = p
	return f
}

// snapshot returns every file under .termino keyed by relative path.
func (f *fixture) snapshot() map[string]string {
	out := map[string]string{}
	dir := filepath.Join(f.root, sharefile.Dir)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		data, _ := os.ReadFile(path)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

// changed lists the paths added, removed or modified between two snapshots.
func changed(before, after map[string]string) []string {
	var out []string
	for p, v := range after {
		if old, ok := before[p]; !ok || old != v {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}

func (f *fixture) mutate(fn func()) []string {
	f.t.Helper()
	before := f.snapshot()
	fn()
	return changed(before, f.snapshot())
}

func (f *fixture) taskFile(id string) sharefile.TaskFile {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, sharefile.Dir, sharefile.TaskFilePath(id)))
	if err != nil {
		f.t.Fatalf("read task file: %v", err)
	}
	tf, err := shareinfra.TOMLCodec{}.DecodeTask(data)
	if err != nil {
		f.t.Fatalf("decode task file: %v", err)
	}
	return tf
}

func (f *fixture) projectFile() sharefile.ProjectFile {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, sharefile.Dir, sharefile.ProjectFileName))
	if err != nil {
		f.t.Fatalf("read project file: %v", err)
	}
	pf, err := shareinfra.TOMLCodec{}.DecodeProject(data)
	if err != nil {
		f.t.Fatalf("decode project file: %v", err)
	}
	return pf
}

func (f *fixture) column(name string) string {
	cols, _ := f.board.GetBoard(f.ctx, f.project.ID)
	for _, c := range cols.Columns {
		if c.Name == name {
			return c.ID
		}
	}
	f.t.Fatalf("no column %q", name)
	return ""
}

func (f *fixture) createTask(title string) taskdomain.Task {
	f.t.Helper()
	task, err := f.tasks.CreateTask(f.ctx, actor, taskapp.CreateTaskInput{ProjectID: f.project.ID, Title: title})
	if err != nil {
		f.t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func expectOne(t *testing.T, op string, got []string, want string) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Errorf("%s changed %v, want exactly [%s]", op, got, want)
	}
}

func TestInit_ExportsProjectFile(t *testing.T) {
	f := newFixture(t, true)
	pf := f.projectFile()
	if pf.Key != "PROJ" || pf.Name != "termino" || pf.Methodology != "kanban" || len(pf.Columns) != 3 {
		t.Errorf("project file = %+v", pf)
	}
	for _, m := range pf.Members {
		if m.Name == "CLI" || m.Name == "System" {
			t.Errorf("built-in member %q exported", m.Name)
		}
	}
	if f.project.ShareRoot != f.root || f.project.AssociatedPath != f.root {
		t.Errorf("Init did not store the share root: %+v", f.project)
	}
}

func TestExport_NotSharedWritesNothing(t *testing.T) {
	f := newFixture(t, false)
	task := f.createTask("local only")
	_ = f.tasks.MoveTask(f.ctx, actor, task.ID, f.column("Done"), 1)
	if _, err := os.Stat(filepath.Join(f.root, sharefile.Dir)); !os.IsNotExist(err) {
		t.Errorf(".termino exists for a project without share root: %v", err)
	}
}

func TestExport_SingleFileMutations(t *testing.T) {
	f := newFixture(t, true)
	var task taskdomain.Task
	got := f.mutate(func() { task = f.createTask("First") })
	path := sharefile.TaskFilePath(task.ID)
	expectOne(t, "create", got, path)
	tf := f.taskFile(task.ID)
	if tf.Number != 1 || tf.Title != "First" || tf.Column != "Backlog" || tf.UpdatedBy != "Ana" || tf.Priority != "medium" {
		t.Errorf("task file = %+v", tf)
	}

	title := "Renamed"
	expectOne(t, "update", f.mutate(func() {
		if err := f.tasks.UpdateTask(f.ctx, actor, taskapp.UpdateTaskInput{TaskID: task.ID, Title: &title}); err != nil {
			t.Fatal(err)
		}
	}), path)
	expectOne(t, "move", f.mutate(func() {
		if err := f.tasks.MoveTask(f.ctx, actor, task.ID, f.column("Done"), 4); err != nil {
			t.Fatal(err)
		}
	}), path)
	if tf := f.taskFile(task.ID); tf.Column != "Done" || tf.Position != 4 || tf.Title != "Renamed" {
		t.Errorf("after move: %+v", tf)
	}

	m, err := f.members.CreateMember(f.ctx, actor, "Bob", memberdomain.KindHuman)
	if err != nil {
		t.Fatal(err)
	}
	expectOne(t, "assign", f.mutate(func() {
		if err := f.tasks.AssignTask(f.ctx, actor, task.ID, &m.ID); err != nil {
			t.Fatal(err)
		}
	}), path)
	if tf := f.taskFile(task.ID); tf.Assignee != "Bob" {
		t.Errorf("assignee = %q, want the member name", tf.Assignee)
	}

	if err := f.tasks.AddLabels(f.ctx, actor, task.ID, []string{"ui"}); err != nil {
		t.Fatal(err)
	}
	other := f.createTask("Second")
	expectOne(t, "label", f.mutate(func() {
		if err := f.tasks.AddLabels(f.ctx, actor, other.ID, []string{"UI"}); err != nil {
			t.Fatal(err)
		}
	}), sharefile.TaskFilePath(other.ID))
	if tf := f.taskFile(other.ID); len(tf.Labels) != 1 || tf.Labels[0] != "ui" {
		t.Errorf("labels = %v", tf.Labels)
	}
	if pf := f.projectFile(); len(pf.Labels) != 1 || pf.Labels[0].Name != "ui" {
		t.Errorf("project labels = %+v", pf.Labels)
	}
}

func TestExport_DeleteRemovesAndRestoreRecreates(t *testing.T) {
	f := newFixture(t, true)
	task := f.createTask("Doomed")
	path := sharefile.TaskFilePath(task.ID)
	expectOne(t, "delete", f.mutate(func() {
		if err := f.tasks.DeleteTask(f.ctx, actor, task.ID, true); err != nil {
			t.Fatal(err)
		}
	}), path)
	if _, ok := f.snapshot()[path]; ok {
		t.Fatal("task file still exists after delete")
	}
	if _, ok, _ := shareinfra.NewHashRepository(f.db).Get(f.ctx, f.project.ID, path); ok {
		t.Error("hash still stored after delete")
	}
	expectOne(t, "restore", f.mutate(func() {
		if err := f.tasks.RestoreTask(f.ctx, actor, task.ID); err != nil {
			t.Fatal(err)
		}
	}), path)
	if tf := f.taskFile(task.ID); tf.Title != "Doomed" {
		t.Errorf("restored file = %+v", tf)
	}
}

func TestExport_HashStoredWithWrite(t *testing.T) {
	f := newFixture(t, true)
	task := f.createTask("Hashed")
	path := sharefile.TaskFilePath(task.ID)
	h, ok, err := shareinfra.NewHashRepository(f.db).Get(f.ctx, f.project.ID, path)
	if err != nil || !ok || h != sharefile.ContentHash([]byte(f.snapshot()[path])) {
		t.Errorf("hash = %q %v %v", h, ok, err)
	}
}

func TestExport_RenameColumnRewritesReferencingFiles(t *testing.T) {
	f := newFixture(t, true)
	inBacklog := f.createTask("In backlog")
	inDone := f.createTask("In done")
	if err := f.tasks.MoveTask(f.ctx, actor, inDone.ID, f.column("Done"), 1); err != nil {
		t.Fatal(err)
	}
	got := f.mutate(func() {
		if err := f.board.RenameColumn(f.ctx, actor, f.column("Backlog"), "Todo"); err != nil {
			t.Fatal(err)
		}
	})
	if len(got) != 2 || !contains(got, sharefile.ProjectFileName) || !contains(got, sharefile.TaskFilePath(inBacklog.ID)) {
		t.Errorf("rename column changed %v", got)
	}
	if tf := f.taskFile(inBacklog.ID); tf.Column != "Todo" {
		t.Errorf("column = %q", tf.Column)
	}
}

func TestExport_RenameMemberRewritesReferencingFiles(t *testing.T) {
	f := newFixture(t, true)
	m, err := f.members.CreateMember(f.ctx, actor, "Bob", memberdomain.KindHuman)
	if err != nil {
		t.Fatal(err)
	}
	task := f.createTask("Assigned")
	_ = f.createTask("Unassigned")
	if err := f.tasks.AssignTask(f.ctx, actor, task.ID, &m.ID); err != nil {
		t.Fatal(err)
	}
	got := f.mutate(func() {
		m.Name = "Robert"
		if err := f.memberDB.Save(f.ctx, m); err != nil {
			t.Fatal(err)
		}
	})
	if len(got) != 2 || !contains(got, sharefile.ProjectFileName) || !contains(got, sharefile.TaskFilePath(task.ID)) {
		t.Errorf("rename member changed %v", got)
	}
	if tf := f.taskFile(task.ID); tf.Assignee != "Robert" {
		t.Errorf("assignee = %q", tf.Assignee)
	}
}

func TestExport_SprintRenameRewritesReferencingFiles(t *testing.T) {
	f := newFixture(t, true)
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	s, err := f.sprints.CreateSprint(f.ctx, actor, f.project.ID, "Sprint 1", start, start.AddDate(0, 0, 14))
	if err != nil {
		t.Fatal(err)
	}
	task := f.createTask("Planned")
	expectOne(t, "add to sprint", f.mutate(func() {
		if err := f.sprints.AddTaskToSprint(f.ctx, actor, task.ID, s.ID); err != nil {
			t.Fatal(err)
		}
	}), sharefile.TaskFilePath(task.ID))
	if tf := f.taskFile(task.ID); tf.Sprint != "Sprint 1" {
		t.Errorf("sprint = %q", tf.Sprint)
	}
	pf := f.projectFile()
	if len(pf.Sprints) != 1 || !pf.Sprints[0].Start.Equal(start) || pf.Sprints[0].State != "draft" {
		t.Errorf("sprints = %+v", pf.Sprints)
	}
	got := f.mutate(func() {
		s.Name = "Sprint One"
		if err := f.svc.Sprints(sprintinfra.NewSprintRepository(f.db)).Save(f.ctx, s); err != nil {
			t.Fatal(err)
		}
	})
	if len(got) != 2 {
		t.Errorf("rename sprint changed %v", got)
	}
}

func TestExport_FailureIsLoggedNotReturned(t *testing.T) {
	f := newFixture(t, true)
	// Turn .termino/tasks into a regular file so every task write fails.
	tasksDir := filepath.Join(f.root, sharefile.Dir, sharefile.TasksDir)
	_ = os.RemoveAll(tasksDir)
	if err := os.WriteFile(tasksDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tasks.CreateTask(f.ctx, actor, taskapp.CreateTaskInput{ProjectID: f.project.ID, Title: "Still saved"}); err != nil {
		t.Fatalf("mutation failed because of export: %v", err)
	}
	if !strings.Contains(f.logs.String(), "share export failed") {
		t.Errorf("export failure not logged: %q", f.logs.String())
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
