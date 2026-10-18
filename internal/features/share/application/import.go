package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/features/share/domain"
	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
)

// ImportResult summarizes one import of a shared project.
type ImportResult struct {
	// Changed counts the tasks created, updated, renumbered or deleted, plus
	// one when project.toml was applied. Zero means the database is as it was.
	Changed int
	// Skipped lists the files that could not be applied, with the reason.
	Skipped []domain.Skipped
}

// reasonNoProjectFile is recorded when .termino/project.toml is missing
// (e.g. a branch without it is checked out). Nothing is imported then, so a
// missing directory never soft-deletes every task.
const reasonNoProjectFile = "project.toml is missing; nothing was imported"

var projectKeyRe = regexp.MustCompile(`^[A-Z]{2,5}$`)

// Import applies the shared files of a project to the local database: it
// reads every file under <share_root>/.termino whose hash differs from the
// one recorded in share_files, upserts the project's columns, members,
// labels and sprints by name, upserts tasks keeping their exported
// timestamps, soft-deletes tasks whose file vanished and resolves duplicate
// numbers with domain.LosesNumber. Invalid files, files with conflict
// markers and files from a newer format are skipped and recorded.
//
// Import writes through the undecorated repositories, so it never triggers
// an export; the only files it writes are those of renumbered tasks. It is
// idempotent: a second run over unchanged files changes nothing.
func (s *ShareService) Import(ctx context.Context, projectID string) (ImportResult, error) {
	p, ok, err := s.sharedProject(ctx, projectID)
	if err != nil || !ok {
		return ImportResult{}, err
	}
	run := &importRun{s: s, p: p}
	err = run.run(ctx)
	if s.d.State != nil {
		if rerr := s.d.State.RecordImport(ctx, p.ID, s.now(), run.res.Skipped); rerr != nil {
			err = errors.Join(err, rerr)
		}
	}
	return run.res, err
}

// Status returns the state recorded by the project's last import.
func (s *ShareService) Status(ctx context.Context, projectID string) (domain.ImportStatus, error) {
	if s.d.State == nil {
		return domain.ImportStatus{}, nil
	}
	return s.d.State.LastImport(ctx, projectID)
}

func (s *ShareService) now() time.Time {
	if s.d.Now != nil {
		return s.d.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *ShareService) logger() *slog.Logger {
	if s.d.Logger != nil {
		if l := s.d.Logger(); l != nil {
			return l
		}
	}
	return slog.New(slog.DiscardHandler)
}

// importRun holds the state of one import.
type importRun struct {
	s       *ShareService
	p       projectdomain.Project
	hashes  map[string]string
	res     ImportResult
	cols    []boarddomain.Column
	members []memberdomain.Member
	labels  []taskdomain.Label
	sprints []sprintdomain.Sprint
}

// pendingTask is a changed task file that decoded.
type pendingTask struct {
	rel  string
	hash string
	f    domain.TaskFile
}

func (r *importRun) skip(rel string, err error) {
	r.res.Skipped = append(r.res.Skipped, domain.Skipped{Path: rel, Reason: err.Error()})
}

func (r *importRun) run(ctx context.Context) error {
	d := r.s.d
	hashes, err := d.Hashes.List(ctx, r.p.ID)
	if err != nil {
		return err
	}
	r.hashes = hashes
	data, err := d.Files.Read(absPath(r.p, domain.ProjectFileName))
	if errors.Is(err, os.ErrNotExist) {
		r.res.Skipped = append(r.res.Skipped, domain.Skipped{Path: domain.ProjectFileName, Reason: reasonNoProjectFile})
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.loadRefs(ctx); err != nil {
		return err
	}
	if hash := domain.ContentHash(data); hash != r.hashes[domain.ProjectFileName] {
		f, err := d.Codec.DecodeProject(data)
		if err != nil {
			r.skip(domain.ProjectFileName, err)
		} else {
			if err := r.applyProject(ctx, f); err != nil {
				return err
			}
			if err := d.Hashes.Put(ctx, r.p.ID, domain.ProjectFileName, hash); err != nil {
				return err
			}
			r.res.Changed++
		}
	}
	pending, present, err := r.readTasks()
	if err != nil {
		return err
	}
	if err := r.deleteVanished(ctx, present); err != nil {
		return err
	}
	return r.applyTasks(ctx, pending)
}

func (r *importRun) loadRefs(ctx context.Context) error {
	d := r.s.d
	var err error
	if r.cols, err = d.Board.ListColumns(ctx, r.p.ID); err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	if r.members, err = d.Members.List(ctx); err != nil {
		return fmt.Errorf("list members: %w", err)
	}
	if r.labels, err = d.Labels.ListByProject(ctx, r.p.ID); err != nil {
		return fmt.Errorf("list labels: %w", err)
	}
	if r.sprints, err = d.Sprints.ListByProject(ctx, r.p.ID); err != nil {
		return fmt.Errorf("list sprints: %w", err)
	}
	return nil
}

// applyProject upserts the project fields and every column, member, label
// and sprint of project.toml. Entities missing from the file are kept: they
// may still be referenced locally.
func (r *importRun) applyProject(ctx context.Context, f domain.ProjectFile) error {
	d := r.s.d
	p := r.p
	if name := sanitize.Line(f.Name); name != "" {
		p.Name = name
	}
	if m := projectdomain.MethodologyKind(f.Methodology); m == projectdomain.MethodologyKanban || m == projectdomain.MethodologyScrum {
		p.Methodology = m
	}
	if f.Key != p.Key && projectKeyRe.MatchString(f.Key) {
		if other, err := d.Projects.FindByKey(ctx, f.Key); err == nil && other.ID != p.ID {
			r.s.logger().Warn("share import: key already used by another project", "key", f.Key, "project", p.ID)
		} else {
			p.Key = f.Key
		}
	}
	if p != r.p {
		if err := d.Projects.Save(ctx, p); err != nil {
			return fmt.Errorf("save project: %w", err)
		}
		r.p = p
	}
	for _, c := range f.Columns {
		if err := r.applyColumn(ctx, c); err != nil {
			return err
		}
	}
	for _, m := range f.Members {
		kind := memberdomain.Kind(m.Kind)
		if kind != memberdomain.KindAgent {
			kind = memberdomain.KindHuman
		}
		if _, err := r.member(ctx, m.Name, kind); err != nil {
			return err
		}
	}
	for _, l := range f.Labels {
		if _, err := r.label(ctx, l.Name, l.Color); err != nil {
			return err
		}
	}
	for _, sp := range f.Sprints {
		if err := r.applySprint(ctx, sp); err != nil {
			return err
		}
	}
	return nil
}

func (r *importRun) applyColumn(ctx context.Context, e domain.ColumnEntry) error {
	name := sanitize.Line(e.Name)
	if name == "" {
		return nil
	}
	var wip *int
	if e.WIPLimit > 0 {
		w := e.WIPLimit
		wip = &w
	}
	for i, c := range r.cols {
		if !strings.EqualFold(c.Name, name) {
			continue
		}
		if c.Position == e.Position && intPtrEq(c.WIPLimit, wip) {
			return nil
		}
		c.Position, c.WIPLimit = e.Position, wip
		if err := r.s.d.Board.SaveColumn(ctx, c); err != nil {
			return fmt.Errorf("save column: %w", err)
		}
		r.cols[i] = c
		return nil
	}
	c := boarddomain.Column{ID: uuid.NewString(), ProjectID: r.p.ID, Name: name, Position: e.Position, WIPLimit: wip}
	if err := r.s.d.Board.SaveColumn(ctx, c); err != nil {
		return fmt.Errorf("create column: %w", err)
	}
	r.cols = append(r.cols, c)
	return nil
}

func (r *importRun) applySprint(ctx context.Context, e domain.SprintEntry) error {
	name := sanitize.Line(e.Name)
	if name == "" {
		return nil
	}
	state := sprintdomain.SprintState(e.State)
	switch state {
	case sprintdomain.SprintStateDraft, sprintdomain.SprintStateActive, sprintdomain.SprintStateClosed:
	default:
		state = sprintdomain.SprintStateDraft
	}
	for i, sp := range r.sprints {
		if !strings.EqualFold(sp.Name, name) {
			continue
		}
		if dateOnly(sp.StartsAt).Equal(e.Start) && dateOnly(sp.EndsAt).Equal(e.End) && sp.State == state {
			return nil
		}
		sp.StartsAt, sp.EndsAt, sp.State = e.Start, e.End, state
		if err := r.s.d.Sprints.Save(ctx, sp); err != nil {
			// e.g. a second active sprint: keep the local one and go on.
			r.s.logger().Warn("share import: sprint not updated", "sprint", name, "error", err.Error())
			return nil
		}
		r.sprints[i] = sp
		return nil
	}
	sp := sprintdomain.Sprint{ID: uuid.NewString(), ProjectID: r.p.ID, Name: name, StartsAt: e.Start, EndsAt: e.End, State: state}
	if err := r.s.d.Sprints.Save(ctx, sp); err != nil {
		r.s.logger().Warn("share import: sprint not created", "sprint", name, "error", err.Error())
		return nil
	}
	r.sprints = append(r.sprints, sp)
	return nil
}

// column returns the ID of the column named name, creating it at the end of
// the board when unknown. An empty name means the first column.
func (r *importRun) column(ctx context.Context, name string) (string, error) {
	name = sanitize.Line(name)
	maxPos, first := -1, -1
	for i, c := range r.cols {
		if name != "" && strings.EqualFold(c.Name, name) {
			return c.ID, nil
		}
		if first < 0 || c.Position < r.cols[first].Position {
			first = i
		}
		maxPos = max(maxPos, c.Position)
	}
	if name == "" {
		if first >= 0 {
			return r.cols[first].ID, nil
		}
		name = "Backlog"
	}
	c := boarddomain.Column{ID: uuid.NewString(), ProjectID: r.p.ID, Name: name, Position: maxPos + 1}
	if err := r.s.d.Board.SaveColumn(ctx, c); err != nil {
		return "", fmt.Errorf("create column %q: %w", name, err)
	}
	r.cols = append(r.cols, c)
	return c.ID, nil
}

// member returns the ID of the member named name (members are global),
// adding it with kind when unknown. An empty name yields nil.
func (r *importRun) member(ctx context.Context, name string, kind memberdomain.Kind) (*string, error) {
	name = sanitize.Line(name)
	if name == "" {
		return nil, nil
	}
	for _, m := range r.members {
		if !m.System() && strings.EqualFold(m.Name, name) {
			id := m.ID
			return &id, nil
		}
	}
	m := memberdomain.Member{ID: uuid.NewString(), Name: name, Kind: kind, CreatedAt: r.s.now()}
	if err := r.s.d.Members.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("add member %q: %w", name, err)
	}
	r.members = append(r.members, m)
	return &m.ID, nil
}

// label returns the ID of the project label named name, creating it with
// color when unknown.
func (r *importRun) label(ctx context.Context, name, color string) (string, error) {
	name = sanitize.Line(name)
	for _, l := range r.labels {
		if strings.EqualFold(l.Name, name) {
			return l.ID, nil
		}
	}
	l := taskdomain.Label{ID: uuid.NewString(), ProjectID: r.p.ID, Name: name, Color: sanitize.Line(color)}
	if err := r.s.d.Labels.Save(ctx, l); err != nil {
		return "", fmt.Errorf("create label %q: %w", name, err)
	}
	r.labels = append(r.labels, l)
	return l.ID, nil
}

// sprint returns the ID of the project sprint named name, creating it as a
// draft when unknown. An empty name yields nil.
func (r *importRun) sprint(ctx context.Context, name string) (*string, error) {
	name = sanitize.Line(name)
	if name == "" {
		return nil, nil
	}
	for _, sp := range r.sprints {
		if strings.EqualFold(sp.Name, name) {
			id := sp.ID
			return &id, nil
		}
	}
	sp := sprintdomain.Sprint{ID: uuid.NewString(), ProjectID: r.p.ID, Name: name, State: sprintdomain.SprintStateDraft}
	if err := r.s.d.Sprints.Save(ctx, sp); err != nil {
		return nil, fmt.Errorf("create sprint %q: %w", name, err)
	}
	r.sprints = append(r.sprints, sp)
	return &sp.ID, nil
}

// readTasks decodes every task file whose content changed since it was last
// written or read. present holds the relative path of every task file on
// disk, changed or not.
func (r *importRun) readTasks() ([]pendingTask, map[string]bool, error) {
	d := r.s.d
	names, err := d.Files.List(filepath.Join(r.p.ShareRoot, domain.Dir, domain.TasksDir))
	if err != nil {
		return nil, nil, err
	}
	present := map[string]bool{}
	var pending []pendingTask
	for _, name := range names {
		id, ok := strings.CutSuffix(name, ".toml")
		if !ok {
			continue
		}
		rel := domain.TaskFilePath(id)
		present[rel] = true
		data, err := d.Files.Read(absPath(r.p, rel))
		if err != nil {
			r.skip(rel, err)
			continue
		}
		hash := domain.ContentHash(data)
		if hash == r.hashes[rel] {
			continue
		}
		f, err := d.Codec.DecodeTask(data)
		if err != nil {
			r.skip(rel, err)
			continue
		}
		if f.ID != id {
			r.skip(rel, fmt.Errorf("%w: id %q does not match the file name", domain.ErrInvalidFile, f.ID))
			continue
		}
		pending = append(pending, pendingTask{rel: rel, hash: hash, f: f})
	}
	return pending, present, nil
}

// deleteVanished soft-deletes the tasks whose file was imported or exported
// before and is now gone.
func (r *importRun) deleteVanished(ctx context.Context, present map[string]bool) error {
	d := r.s.d
	prefix := domain.TasksDir + "/"
	rels := make([]string, 0, len(r.hashes))
	for rel := range r.hashes {
		if strings.HasPrefix(rel, prefix) && !present[rel] {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	for _, rel := range rels {
		id := strings.TrimSuffix(path.Base(rel), ".toml")
		t, err := d.Tasks.FindByID(ctx, id)
		switch {
		case errors.Is(err, taskdomain.ErrTaskNotFound):
		case err != nil:
			return fmt.Errorf("find task: %w", err)
		case t.DeletedAt == nil:
			if err := d.Tasks.SoftDelete(ctx, id, r.s.now()); err != nil {
				return fmt.Errorf("delete task: %w", err)
			}
			r.res.Changed++
		}
		if err := d.Hashes.Delete(ctx, r.p.ID, rel); err != nil {
			return err
		}
	}
	return nil
}

// applyTasks upserts the changed task files and resolves duplicate numbers
// in one transaction, then rewrites the files of renumbered tasks.
func (r *importRun) applyTasks(ctx context.Context, pending []pendingTask) error {
	if len(pending) == 0 {
		return nil
	}
	d := r.s.d
	all, err := d.Tasks.List(ctx, taskdomain.TaskFilter{ProjectID: r.p.ID, IncludeDeleted: true})
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	current := make(map[string]taskdomain.Task, len(all))
	for _, t := range all {
		current[t.ID] = t
	}
	numbers := r.resolveNumbers(all, pending)

	tasks := make([]taskdomain.Task, 0, len(pending))
	labelIDs := map[string][]string{}
	fileNumber := map[string]int{}
	applied := make([]pendingTask, 0, len(pending))
	for _, pt := range pending {
		t, ids, err := r.buildTask(ctx, pt.f, current)
		if err != nil {
			r.skip(pt.rel, err)
			continue
		}
		t.Number = numbers[t.ID]
		tasks = append(tasks, t)
		labelIDs[t.ID] = ids
		fileNumber[t.ID] = pt.f.Number
		applied = append(applied, pt)
	}
	renumber := map[string]int{}
	for id, n := range numbers {
		if _, imported := fileNumber[id]; !imported && current[id].Number != n {
			renumber[id] = n
		}
	}
	if len(tasks) == 0 && len(renumber) == 0 {
		return nil
	}
	if err := d.Tasks.ApplyImported(ctx, renumber, tasks); err != nil {
		return fmt.Errorf("apply imported tasks: %w", err)
	}
	r.res.Changed += len(tasks) + len(renumber)
	for _, t := range tasks {
		if err := d.Labels.SetTaskLabels(ctx, t.ID, labelIDs[t.ID]); err != nil {
			return fmt.Errorf("set task labels: %w", err)
		}
	}

	// Renumbered tasks get their file rewritten; every other imported file
	// is recorded as read.
	refs, err := r.s.loadRefs(ctx, r.p.ID)
	if err != nil {
		return err
	}
	var errs []error
	for _, pt := range applied {
		if numbers[pt.f.ID] == fileNumber[pt.f.ID] {
			errs = append(errs, d.Hashes.Put(ctx, r.p.ID, pt.rel, pt.hash))
			continue
		}
		renumber[pt.f.ID] = numbers[pt.f.ID]
	}
	ids := make([]string, 0, len(renumber))
	for id := range renumber {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t, err := d.Tasks.FindByID(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("find task: %w", err))
			continue
		}
		errs = append(errs, r.s.exportTask(ctx, r.p, refs, t))
	}
	return errors.Join(errs...)
}

// resolveNumbers returns the number every task of the project ends up with:
// its stored number, or the one in its changed file. When several tasks
// claim one number the task that domain.LosesNumber says keeps it does, and
// the others take the next numbers above the highest one claimed, in a
// deterministic order. Machines that see the same files therefore converge.
func (r *importRun) resolveNumbers(all []taskdomain.Task, pending []pendingTask) map[string]int {
	claim := map[string]domain.TaskFile{}
	numbers := map[string]int{}
	for _, t := range all {
		claim[t.ID] = domain.TaskFile{ID: t.ID, CreatedAt: t.CreatedAt}
		numbers[t.ID] = t.Number
	}
	for _, pt := range pending {
		claim[pt.f.ID] = domain.TaskFile{ID: pt.f.ID, CreatedAt: pt.f.CreatedAt}
		numbers[pt.f.ID] = pt.f.Number
	}
	byNumber := map[int][]string{}
	highest := 0
	for id, n := range numbers {
		byNumber[n] = append(byNumber[n], id)
		highest = max(highest, n)
	}
	var losers []domain.TaskFile
	for n, ids := range byNumber {
		claimants := make([]domain.TaskFile, 0, len(ids))
		for _, id := range ids {
			claimants = append(claimants, claim[id])
		}
		sort.Slice(claimants, func(i, j int) bool { return domain.LosesNumber(claimants[j], claimants[i]) })
		if n > 0 {
			claimants = claimants[1:]
		}
		losers = append(losers, claimants...)
	}
	sort.Slice(losers, func(i, j int) bool {
		if numbers[losers[i].ID] != numbers[losers[j].ID] {
			return numbers[losers[i].ID] < numbers[losers[j].ID]
		}
		return domain.LosesNumber(losers[j], losers[i])
	})
	for _, l := range losers {
		highest++
		numbers[l.ID] = highest
	}
	return numbers
}

// buildTask turns a task file into the task to store, keeping the local-only
// fields (task source) of the stored task and creating the column, member,
// labels and sprint it names when they are unknown.
func (r *importRun) buildTask(ctx context.Context, f domain.TaskFile, current map[string]taskdomain.Task) (taskdomain.Task, []string, error) {
	t := current[f.ID]
	t.ID, t.ProjectID = f.ID, r.p.ID
	t.Title = sanitize.Line(f.Title)
	t.Description = sanitize.Clean(f.Description)
	t.Position = f.Position
	t.Points = f.Points
	t.Priority = taskdomain.Priority(f.Priority)
	if !t.Priority.Valid() {
		t.Priority = taskdomain.PriorityMedium
	}
	t.DueDate = f.Due
	t.DeletedAt = nil
	t.CreatedAt, t.UpdatedAt = f.CreatedAt, f.UpdatedAt
	if t.CreatedAt.IsZero() {
		t.CreatedAt = r.s.now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}
	t.UpdatedBy = sanitize.Line(f.UpdatedBy)
	var err error
	if t.ColumnID, err = r.column(ctx, f.Column); err != nil {
		return t, nil, err
	}
	if t.AssigneeID, err = r.member(ctx, f.Assignee, memberdomain.KindHuman); err != nil {
		return t, nil, err
	}
	if t.SprintID, err = r.sprint(ctx, f.Sprint); err != nil {
		return t, nil, err
	}
	var ids []string
	t.Labels = nil
	for _, name := range f.Labels {
		if sanitize.Line(name) == "" {
			continue
		}
		id, err := r.label(ctx, name, "")
		if err != nil {
			return t, nil, err
		}
		ids = append(ids, id)
	}
	return t, ids, nil
}

func intPtrEq(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
