// Package application contains the repository-sharing use cases: exporting a
// project and its tasks to <share_root>/.termino/ after every local change.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/features/share/domain"
	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// LabelStore lists a project's labels and, for import, creates labels and
// sets the exact labels of a task.
type LabelStore interface {
	ListByProject(ctx context.Context, projectID string) ([]taskdomain.Label, error)
	Save(ctx context.Context, l taskdomain.Label) error
	SetTaskLabels(ctx context.Context, taskID string, labelIDs []string) error
}

// TaskStore is the task repository plus the import-only write that keeps
// the timestamps and numbers read from shared files.
type TaskStore interface {
	taskdomain.TaskRepository
	ApplyImported(ctx context.Context, renumber map[string]int, tasks []taskdomain.Task) error
}

// ImportState records the outcome of the last import of each project.
type ImportState interface {
	RecordImport(ctx context.Context, projectID string, at time.Time, skipped []domain.Skipped) error
	LastImport(ctx context.Context, projectID string) (domain.ImportStatus, error)
	// SetOptOut records or forgets that a project stopped sharing from root.
	SetOptOut(ctx context.Context, projectID, root string, on bool) error
	OptedOut(ctx context.Context, projectID, root string) (bool, error)
}

// FileStore reads and writes shared files by absolute path.
type FileStore interface {
	Write(path string, data []byte) error
	Read(path string) ([]byte, error)
	Remove(path string) error
	List(dir string) ([]string, error)
}

// HashStore keeps the content hash of every shared file last written or read,
// keyed by project and path relative to the .termino directory.
type HashStore interface {
	Get(ctx context.Context, projectID, path string) (string, bool, error)
	Put(ctx context.Context, projectID, path, hash string) error
	Delete(ctx context.Context, projectID, path string) error
	List(ctx context.Context, projectID string) (map[string]string, error)
}

// Codec converts snapshots to and from file content.
type Codec interface {
	EncodeProject(p domain.ProjectFile) []byte
	EncodeTask(t domain.TaskFile) []byte
	DecodeProject(data []byte) (domain.ProjectFile, error)
	DecodeTask(data []byte) (domain.TaskFile, error)
}

// Deps are the ports ShareService depends on. The repositories must be the
// undecorated ones, so reading state for an export never triggers another.
type Deps struct {
	Projects projectdomain.ProjectRepository
	Board    boarddomain.BoardRepository
	Tasks    TaskStore
	Members  memberdomain.MemberRepository
	Labels   LabelStore
	Sprints  sprintdomain.SprintRepository
	Files    FileStore
	Hashes   HashStore
	Codec    Codec
	State    ImportState
	// Now returns the current time; nil uses time.Now.
	Now func() time.Time
	// Logger returns the logger for export failures; it is resolved lazily so
	// the log file is only opened when something goes wrong. nil discards.
	Logger func() *slog.Logger
}

// ShareService exports shared projects to their .termino directory.
type ShareService struct {
	d Deps
}

// NewShareService constructs a ShareService.
func NewShareService(d Deps) *ShareService {
	return &ShareService{d: d}
}

// Init starts sharing a project from root: it stores root as the project's
// share root (and as its associated path when that is empty) and exports
// everything.
func (s *ShareService) Init(ctx context.Context, projectID, root string) error {
	p, err := s.d.Projects.FindByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	p.ShareRoot = root
	if p.AssociatedPath == "" {
		p.AssociatedPath = root
	}
	if err := s.d.Projects.Save(ctx, p); err != nil {
		return fmt.Errorf("save project: %w", err)
	}
	if s.d.State != nil {
		if err := s.d.State.SetOptOut(ctx, projectID, root, false); err != nil {
			return err
		}
	}
	return s.ExportAll(ctx, projectID)
}

// sharedProject returns the project when it has a share root; ok is false for
// a project that is not shared (or no longer exists).
func (s *ShareService) sharedProject(ctx context.Context, projectID string) (projectdomain.Project, bool, error) {
	p, err := s.d.Projects.FindByID(ctx, projectID)
	if err != nil {
		return projectdomain.Project{}, false, fmt.Errorf("find project: %w", err)
	}
	return p, p.ShareRoot != "", nil
}

// ExportProject rewrites project.toml of a shared project.
func (s *ShareService) ExportProject(ctx context.Context, projectID string) error {
	p, ok, err := s.sharedProject(ctx, projectID)
	if err != nil || !ok {
		return err
	}
	return s.exportProject(ctx, p)
}

// ExportTask rewrites the file of one task of a shared project, or removes it
// when the task is soft-deleted.
func (s *ShareService) ExportTask(ctx context.Context, taskID string) error {
	t, err := s.d.Tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	p, ok, err := s.sharedProject(ctx, t.ProjectID)
	if err != nil || !ok {
		return err
	}
	r, err := s.loadRefs(ctx, p.ID)
	if err != nil {
		return err
	}
	return s.exportTask(ctx, p, r, t)
}

// RemoveTask deletes the file of a task of a shared project.
func (s *ShareService) RemoveTask(ctx context.Context, projectID, taskID string) error {
	p, ok, err := s.sharedProject(ctx, projectID)
	if err != nil || !ok {
		return err
	}
	return s.remove(ctx, p, domain.TaskFilePath(taskID))
}

// ExportAll rewrites project.toml and every task file of a shared project.
// Files whose content is unchanged are left untouched, so a rename only
// rewrites the files that reference the renamed value.
func (s *ShareService) ExportAll(ctx context.Context, projectID string) error {
	p, ok, err := s.sharedProject(ctx, projectID)
	if err != nil || !ok {
		return err
	}
	if err := s.exportProject(ctx, p); err != nil {
		return err
	}
	tasks, err := s.d.Tasks.List(ctx, taskdomain.TaskFilter{ProjectID: p.ID, IncludeDeleted: true})
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	r, err := s.loadRefs(ctx, p.ID)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range tasks {
		errs = append(errs, s.exportTask(ctx, p, r, t))
	}
	return errors.Join(errs...)
}

// ExportAllShared runs ExportAll for every shared project. Members are global,
// so a member change affects all of them.
func (s *ShareService) ExportAllShared(ctx context.Context) error {
	projects, err := s.d.Projects.List(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	var errs []error
	for _, p := range projects {
		if p.ShareRoot != "" {
			errs = append(errs, s.ExportAll(ctx, p.ID))
		}
	}
	return errors.Join(errs...)
}

func (s *ShareService) exportProject(ctx context.Context, p projectdomain.Project) error {
	f, err := s.projectFile(ctx, p)
	if err != nil {
		return err
	}
	return s.write(ctx, p, domain.ProjectFileName, s.d.Codec.EncodeProject(f))
}

func (s *ShareService) exportTask(ctx context.Context, p projectdomain.Project, r refs, t taskdomain.Task) error {
	if t.DeletedAt != nil {
		return s.remove(ctx, p, domain.TaskFilePath(t.ID))
	}
	return s.write(ctx, p, domain.TaskFilePath(t.ID), s.d.Codec.EncodeTask(r.taskFile(t)))
}

// write stores data at rel (relative to .termino) unless the file already
// holds exactly that content, and records its hash either way.
func (s *ShareService) write(ctx context.Context, p projectdomain.Project, rel string, data []byte) error {
	hash := domain.ContentHash(data)
	abs := absPath(p, rel)
	if cur, err := s.d.Files.Read(abs); err != nil || domain.ContentHash(cur) != hash {
		if err := s.d.Files.Write(abs, data); err != nil {
			return err
		}
	}
	return s.d.Hashes.Put(ctx, p.ID, rel, hash)
}

func (s *ShareService) remove(ctx context.Context, p projectdomain.Project, rel string) error {
	if err := s.d.Files.Remove(absPath(p, rel)); err != nil {
		return err
	}
	return s.d.Hashes.Delete(ctx, p.ID, rel)
}

func absPath(p projectdomain.Project, rel string) string {
	return filepath.Join(p.ShareRoot, domain.Dir, filepath.FromSlash(rel))
}

// projectFile builds the project.toml snapshot. Built-in members (cli,
// system) exist on every machine and are never exported.
func (s *ShareService) projectFile(ctx context.Context, p projectdomain.Project) (domain.ProjectFile, error) {
	f := domain.ProjectFile{Version: domain.FormatVersion, Key: p.Key, Name: p.Name, Methodology: string(p.Methodology)}
	cols, err := s.d.Board.ListColumns(ctx, p.ID)
	if err != nil {
		return f, fmt.Errorf("list columns: %w", err)
	}
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].Position < cols[j].Position })
	for _, c := range cols {
		e := domain.ColumnEntry{Name: c.Name, Position: c.Position}
		if c.WIPLimit != nil {
			e.WIPLimit = *c.WIPLimit
		}
		f.Columns = append(f.Columns, e)
	}
	members, err := s.d.Members.List(ctx)
	if err != nil {
		return f, fmt.Errorf("list members: %w", err)
	}
	for _, m := range members {
		if !m.System() {
			f.Members = append(f.Members, domain.MemberEntry{Name: m.Name, Kind: string(m.Kind)})
		}
	}
	sort.Slice(f.Members, func(i, j int) bool { return lessFold(f.Members[i].Name, f.Members[j].Name) })
	labels, err := s.d.Labels.ListByProject(ctx, p.ID)
	if err != nil {
		return f, fmt.Errorf("list labels: %w", err)
	}
	for _, l := range labels {
		f.Labels = append(f.Labels, domain.LabelEntry{Name: l.Name, Color: l.Color})
	}
	sort.Slice(f.Labels, func(i, j int) bool { return lessFold(f.Labels[i].Name, f.Labels[j].Name) })
	sprints, err := s.d.Sprints.ListByProject(ctx, p.ID)
	if err != nil {
		return f, fmt.Errorf("list sprints: %w", err)
	}
	sort.SliceStable(sprints, func(i, j int) bool {
		if !sprints[i].StartsAt.Equal(sprints[j].StartsAt) {
			return sprints[i].StartsAt.Before(sprints[j].StartsAt)
		}
		return sprints[i].Name < sprints[j].Name
	})
	for _, sp := range sprints {
		f.Sprints = append(f.Sprints, domain.SprintEntry{
			Name: sp.Name, Start: dateOnly(sp.StartsAt), End: dateOnly(sp.EndsAt), State: string(sp.State),
		})
	}
	return f, nil
}

// refs resolves the local IDs a task references to the names exported.
type refs struct {
	columns map[string]string
	members map[string]string
	sprints map[string]string
}

func (s *ShareService) loadRefs(ctx context.Context, projectID string) (refs, error) {
	r := refs{columns: map[string]string{}, members: map[string]string{}, sprints: map[string]string{}}
	cols, err := s.d.Board.ListColumns(ctx, projectID)
	if err != nil {
		return r, fmt.Errorf("list columns: %w", err)
	}
	for _, c := range cols {
		r.columns[c.ID] = c.Name
	}
	members, err := s.d.Members.List(ctx)
	if err != nil {
		return r, fmt.Errorf("list members: %w", err)
	}
	for _, m := range members {
		r.members[m.ID] = m.Name
	}
	sprints, err := s.d.Sprints.ListByProject(ctx, projectID)
	if err != nil {
		return r, fmt.Errorf("list sprints: %w", err)
	}
	for _, sp := range sprints {
		r.sprints[sp.ID] = sp.Name
	}
	return r, nil
}

// taskFile builds the snapshot of t. Comments, activity and task-source
// fields (which hold local absolute paths) are never part of it.
func (r refs) taskFile(t taskdomain.Task) domain.TaskFile {
	f := domain.TaskFile{
		Version: domain.FormatVersion, ID: t.ID, Number: t.Number, Title: t.Title,
		Column: r.columns[t.ColumnID], Position: t.Position, Priority: string(t.Priority),
		Points: t.Points, Labels: t.Labels, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		UpdatedBy: t.UpdatedBy, Description: t.Description,
	}
	if t.AssigneeID != nil {
		f.Assignee = nameOr(r.members, *t.AssigneeID)
	}
	if t.SprintID != nil {
		f.Sprint = nameOr(r.sprints, *t.SprintID)
	}
	if t.DueDate != nil {
		d := dateOnly(*t.DueDate)
		f.Due = &d
	}
	return f
}

// nameOr returns the name for id, or id itself when it is unknown.
func nameOr(names map[string]string, id string) string {
	if n, ok := names[id]; ok {
		return n
	}
	return id
}

func lessFold(a, b string) bool {
	if la, lb := strings.ToLower(a), strings.ToLower(b); la != lb {
		return la < lb
	}
	return a < b
}

// dateOnly truncates t to its UTC calendar date; a zero time stays zero.
func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
