// Package application provides the bidirectional sync service between
// task source files (kirospec, markdown) and the Termino task board.
package application

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/fsnotify/fsnotify"
	sharedomain "github.com/lesquel/termino/internal/domain"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
)

// SyncService synchronises task source files with the Termino task board:
// importing creates cards in the first column, a checked box moves its card
// to Done, and moving a card to Done checks its box (TRD 10.2).
type SyncService struct {
	taskUC  *taskapp.TaskUseCases
	boardUC *boardapp.BoardUseCases
	log     *slog.Logger
}

// NewSyncService constructs a SyncService. It logs through slog.Default()
// until WithLogger supplies a dedicated (file-backed) logger.
func NewSyncService(taskUC *taskapp.TaskUseCases, boardUC *boardapp.BoardUseCases) *SyncService {
	return &SyncService{taskUC: taskUC, boardUC: boardUC, log: slog.Default()}
}

// WithLogger sets the logger used by imports and the watcher. Callers running
// a TUI or stdio protocol must pass a logger that never writes to stdout.
func (s *SyncService) WithLogger(l *slog.Logger) *SyncService {
	if l != nil {
		s.log = l
	}
	return s
}

// ImportFromSource reads all tasks in sourceFile, creates the missing ones in
// the project's first column and moves cards whose checkbox is checked to the
// Done column. Re-importing never duplicates: tasks are matched by source_ref,
// including soft-deleted ones (which are left alone).
// Returns (imported, updated, error).
func (s *SyncService) ImportFromSource(
	ctx context.Context,
	actor sharedomain.Actor,
	projectID string,
	sourceFile tasksourcedomain.SourceFile,
	taskSource tasksourcedomain.TaskSource,
) (imported, updated int, err error) {
	// Refs embed the file path, so normalise it: importing "tasks.md" and
	// "/abs/tasks.md" must address the same cards.
	if abs, absErr := filepath.Abs(sourceFile.Path); absErr == nil {
		sourceFile.Path = abs
	}
	sourceTasks, err := taskSource.Parse(sourceFile)
	if err != nil {
		return 0, 0, fmt.Errorf("parse source file: %w", err)
	}

	existing, err := s.taskUC.ListTasks(ctx, taskdomain.TaskFilter{ProjectID: projectID, IncludeDeleted: true})
	if err != nil {
		return 0, 0, fmt.Errorf("list project tasks: %w", err)
	}
	byRef := make(map[string]taskdomain.Task, len(existing))
	for _, t := range existing {
		if t.SourceRef != nil {
			byRef[*t.SourceRef] = t
		}
	}

	doneCol, err := s.doneColumn(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}

	for _, st := range sourceTasks {
		t, found := byRef[st.Ref]
		if !found {
			title := sanitize.Line(st.Title)
			if title == "" {
				continue
			}
			adapterName, sourcePath, sourceRef := sourceFile.AdapterName, sourceFile.Path, st.Ref
			// ColumnID is left empty: CreateTask places the card in the
			// project's first column.
			created, createErr := s.taskUC.CreateTask(ctx, actor, taskapp.CreateTaskInput{
				ProjectID:   projectID,
				Title:       title,
				Description: sanitize.Clean(st.Description),
				Source:      &adapterName,
				SourcePath:  &sourcePath,
				SourceRef:   &sourceRef,
			})
			if createErr != nil {
				s.log.Warn("sync: create task failed", "ref", st.Ref, "err", createErr)
				continue
			}
			imported++
			t = created
		} else if t.DeletedAt != nil {
			continue
		}

		if st.Done && doneCol != "" && t.ColumnID != doneCol {
			if moveErr := s.moveToEnd(ctx, actor, t.ID, doneCol); moveErr != nil {
				s.log.Warn("sync: move to done failed", "ref", st.Ref, "err", moveErr)
				continue
			}
			if found {
				updated++
			}
		}
	}
	return imported, updated, nil
}

// doneColumn returns the ID of the project's Done column ("" if none).
func (s *SyncService) doneColumn(ctx context.Context, projectID string) (string, error) {
	board, err := s.boardUC.GetBoard(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("load board: %w", err)
	}
	col, ok := boarddomain.DoneColumn(board.Columns)
	if !ok {
		return "", nil
	}
	return col.ID, nil
}

// moveToEnd moves a task to the end of the given column.
func (s *SyncService) moveToEnd(ctx context.Context, actor sharedomain.Actor, taskID, columnID string) error {
	inCol, err := s.taskUC.ListTasks(ctx, taskdomain.TaskFilter{ColumnID: columnID})
	if err != nil {
		return fmt.Errorf("list column tasks: %w", err)
	}
	position := 1.0
	if n := len(inCol); n > 0 {
		position = inCol[n-1].Position + 1
	}
	return s.taskUC.MoveTask(ctx, actor, taskID, columnID, position)
}

// ExportTask writes the task's done state (is it in the Done column?) back
// to its source file. Tasks that were not imported are ignored.
func (s *SyncService) ExportTask(
	ctx context.Context,
	taskID string,
	sources []tasksourcedomain.TaskSource,
) error {
	t, err := s.taskUC.FindTaskByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	if t.SourcePath == nil {
		return nil
	}
	doneCol, err := s.doneColumn(ctx, t.ProjectID)
	if err != nil {
		return err
	}
	return s.ExportToSource(ctx, taskID, sources, []string{doneCol})
}

// ExportToSource updates the source file's checkbox to match the task's done state.
func (s *SyncService) ExportToSource(
	ctx context.Context,
	taskID string,
	sources []tasksourcedomain.TaskSource,
	doneColumnIDs []string,
) error {
	t, err := s.taskUC.FindTaskByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	if t.SourcePath == nil || t.SourceRef == nil || t.Source == nil {
		return nil // no source to export to
	}

	// Determine if the task is "done" based on column membership.
	isDone := false
	for _, doneColID := range doneColumnIDs {
		if t.ColumnID == doneColID {
			isDone = true
			break
		}
	}

	adapter := findSource(sources, *t.Source)
	if adapter == nil {
		return fmt.Errorf("no adapter found for source %q", *t.Source)
	}

	file := tasksourcedomain.SourceFile{
		Path:        *t.SourcePath,
		AdapterName: *t.Source,
	}
	return adapter.WriteStatus(file, *t.SourceRef, isDone)
}

func findSource(sources []tasksourcedomain.TaskSource, name string) tasksourcedomain.TaskSource {
	for _, src := range sources {
		if src.Name() == name {
			return src
		}
	}
	return nil
}

// StartWatcher detects source files under root and watches them; see
// WatchFiles for the lifecycle.
func (s *SyncService) StartWatcher(
	ctx context.Context,
	actor sharedomain.Actor,
	projectID string,
	root string,
	sources []tasksourcedomain.TaskSource,
) (<-chan struct{}, error) {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	var files []tasksourcedomain.SourceFile
	for _, src := range sources {
		detected, err := src.Detect(root)
		if err != nil {
			s.log.Warn("watcher: detect failed", "adapter", src.Name(), "err", err)
			continue
		}
		files = append(files, detected...)
	}
	return s.WatchFiles(ctx, actor, projectID, files, sources)
}

// WatchImported watches the source files the project's tasks were imported
// from, so sync only ever touches files the user imported (TRD 11.8).
func (s *SyncService) WatchImported(
	ctx context.Context,
	actor sharedomain.Actor,
	projectID string,
	sources []tasksourcedomain.TaskSource,
) (<-chan struct{}, error) {
	tasks, err := s.taskUC.ListTasks(ctx, taskdomain.TaskFilter{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("list project tasks: %w", err)
	}
	seen := make(map[string]bool)
	var files []tasksourcedomain.SourceFile
	for _, t := range tasks {
		if t.SourcePath == nil || t.Source == nil || seen[*t.SourcePath] {
			continue
		}
		seen[*t.SourcePath] = true
		files = append(files, tasksourcedomain.SourceFile{Path: *t.SourcePath, AdapterName: *t.Source})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return s.WatchFiles(ctx, actor, projectID, files, sources)
}

// WatchFiles watches files with fsnotify and re-imports a file whenever it is
// written or replaced (atomic rename). It returns once the watches are
// registered; the returned channel closes after ctx is cancelled and the
// watcher has shut down. With nothing to watch the channel is already closed.
func (s *SyncService) WatchFiles(
	ctx context.Context,
	actor sharedomain.Actor,
	projectID string,
	files []tasksourcedomain.SourceFile,
	sources []tasksourcedomain.TaskSource,
) (<-chan struct{}, error) {
	done := make(chan struct{})
	fileToSource := make(map[string]tasksourcedomain.TaskSource)
	for _, f := range files {
		src := findSource(sources, f.AdapterName)
		if src == nil {
			s.log.Warn("watcher: no adapter for file", "file", f.Path, "adapter", f.AdapterName)
			continue
		}
		path := f.Path
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		fileToSource[path] = src
	}
	if len(fileToSource) == 0 {
		close(done)
		return done, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	// Watch directories, not files: editors and atomic writers replace the
	// file via rename, which would drop a file-level watch.
	dirs := make(map[string]bool)
	for path := range fileToSource {
		dir := filepath.Dir(path)
		if dirs[dir] {
			continue
		}
		dirs[dir] = true
		if err := watcher.Add(dir); err != nil {
			s.log.Warn("watcher: add dir failed", "dir", dir, "err", err)
		}
	}

	go func() {
		defer close(done)
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
					continue
				}
				src, found := fileToSource[event.Name]
				if !found {
					continue
				}
				sf := tasksourcedomain.SourceFile{Path: event.Name, AdapterName: src.Name()}
				imported, updated, err := s.ImportFromSource(ctx, actor, projectID, sf, src)
				if err != nil {
					s.log.Error("watcher: import failed", "file", event.Name, "err", err)
				} else if imported > 0 || updated > 0 {
					s.log.Info("watcher: synced", "file", event.Name, "imported", imported, "updated", updated)
				}
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				s.log.Error("watcher error", "err", watchErr)
			}
		}
	}()
	return done, nil
}
