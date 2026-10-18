package application

import (
	"context"
	"log/slog"
	"time"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// The decorators below wrap the repositories the use cases write through.
// Every TUI, CLI and MCP mutation reaches the database through them, so
// exporting after each successful write covers all frontends from one place
// (internal/cli/container.go). Export only runs for projects with a share
// root, and an export failure is logged, never returned: the user's change is
// already saved. Import must use the undecorated repositories so applying a
// shared file never exports it again.

// after runs export once a write succeeded, logging (not returning) its error.
func (s *ShareService) after(ctx context.Context, writeErr error, op string, export func() error) error {
	if writeErr != nil {
		return writeErr
	}
	if err := export(); err != nil {
		logger := slog.New(slog.DiscardHandler)
		if s.d.Logger != nil {
			if l := s.d.Logger(); l != nil {
				logger = l
			}
		}
		logger.LogAttrs(ctx, slog.LevelWarn, "share export failed", slog.String("op", op), slog.String("error", err.Error()))
	}
	return nil
}

// Tasks wraps a task repository so saves, deletes and restores are exported.
func (s *ShareService) Tasks(inner taskdomain.TaskRepository) taskdomain.TaskRepository {
	return &taskRepo{TaskRepository: inner, s: s}
}

type taskRepo struct {
	taskdomain.TaskRepository
	s *ShareService
}

func (r *taskRepo) Save(ctx context.Context, t taskdomain.Task) error {
	return r.s.after(ctx, r.TaskRepository.Save(ctx, t), "task.save", func() error { return r.s.ExportTask(ctx, t.ID) })
}

func (r *taskRepo) SoftDelete(ctx context.Context, id string, at time.Time) error {
	return r.s.after(ctx, r.TaskRepository.SoftDelete(ctx, id, at), "task.delete", func() error { return r.s.ExportTask(ctx, id) })
}

func (r *taskRepo) Restore(ctx context.Context, id string) error {
	return r.s.after(ctx, r.TaskRepository.Restore(ctx, id), "task.restore", func() error { return r.s.ExportTask(ctx, id) })
}

// Labels wraps a label repository: a new label rewrites project.toml and an
// attachment rewrites the task file.
func (s *ShareService) Labels(inner taskdomain.LabelRepository) taskdomain.LabelRepository {
	return &labelRepo{LabelRepository: inner, s: s}
}

type labelRepo struct {
	taskdomain.LabelRepository
	s *ShareService
}

func (r *labelRepo) Save(ctx context.Context, l taskdomain.Label) error {
	return r.s.after(ctx, r.LabelRepository.Save(ctx, l), "label.save", func() error { return r.s.ExportProject(ctx, l.ProjectID) })
}

func (r *labelRepo) Attach(ctx context.Context, taskID, labelID string) error {
	return r.s.after(ctx, r.LabelRepository.Attach(ctx, taskID, labelID), "label.attach", func() error { return r.s.ExportTask(ctx, taskID) })
}

// Board wraps a board repository. Saving a column may rename it, so every
// task file of the project is re-checked (unchanged files are not rewritten).
func (s *ShareService) Board(inner boarddomain.BoardRepository) boarddomain.BoardRepository {
	return &boardRepo{BoardRepository: inner, s: s}
}

type boardRepo struct {
	boarddomain.BoardRepository
	s *ShareService
}

func (r *boardRepo) SaveColumn(ctx context.Context, col boarddomain.Column) error {
	return r.s.after(ctx, r.BoardRepository.SaveColumn(ctx, col), "column.save", func() error { return r.s.ExportAll(ctx, col.ProjectID) })
}

func (r *boardRepo) ReorderColumns(ctx context.Context, projectID string, ids []string) error {
	return r.s.after(ctx, r.BoardRepository.ReorderColumns(ctx, projectID, ids), "column.reorder", func() error { return r.s.ExportProject(ctx, projectID) })
}

func (r *boardRepo) DeleteColumn(ctx context.Context, id string) error {
	col, findErr := r.BoardRepository.FindColumnByID(ctx, id)
	return r.s.after(ctx, r.BoardRepository.DeleteColumn(ctx, id), "column.delete", func() error {
		if findErr != nil {
			return findErr
		}
		return r.s.ExportProject(ctx, col.ProjectID)
	})
}

// Projects wraps a project repository. A saved project may change its key,
// name, methodology or share root, so the whole project is re-exported.
func (s *ShareService) Projects(inner projectdomain.ProjectRepository) projectdomain.ProjectRepository {
	return &projectRepo{ProjectRepository: inner, s: s}
}

type projectRepo struct {
	projectdomain.ProjectRepository
	s *ShareService
}

func (r *projectRepo) Save(ctx context.Context, p projectdomain.Project) error {
	return r.s.after(ctx, r.ProjectRepository.Save(ctx, p), "project.save", func() error { return r.s.ExportAll(ctx, p.ID) })
}

// Members wraps the member repository. Members are global, so a change
// re-exports every shared project.
func (s *ShareService) Members(inner memberdomain.MemberRepository) memberdomain.MemberRepository {
	return &memberRepo{MemberRepository: inner, s: s}
}

type memberRepo struct {
	memberdomain.MemberRepository
	s *ShareService
}

func (r *memberRepo) Save(ctx context.Context, m memberdomain.Member) error {
	return r.s.after(ctx, r.MemberRepository.Save(ctx, m), "member.save", func() error { return r.s.ExportAllShared(ctx) })
}

func (r *memberRepo) Delete(ctx context.Context, id string) error {
	return r.s.after(ctx, r.MemberRepository.Delete(ctx, id), "member.delete", func() error { return r.s.ExportAllShared(ctx) })
}

// Sprints wraps a sprint repository. A saved sprint may be renamed, so the
// whole project is re-checked.
func (s *ShareService) Sprints(inner sprintdomain.SprintRepository) sprintdomain.SprintRepository {
	return &sprintRepo{SprintRepository: inner, s: s}
}

type sprintRepo struct {
	sprintdomain.SprintRepository
	s *ShareService
}

func (r *sprintRepo) Save(ctx context.Context, sp sprintdomain.Sprint) error {
	return r.s.after(ctx, r.SprintRepository.Save(ctx, sp), "sprint.save", func() error { return r.s.ExportAll(ctx, sp.ProjectID) })
}
