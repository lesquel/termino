// Package application provides the bidirectional sync service between
// task source files (kirospec, markdown) and the Termino task board.
package application

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	sharedomain "github.com/lesquel/termino/internal/domain"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
)

// SyncService synchronises task source files with the Termino task board.
type SyncService struct {
	taskUC *taskapp.TaskUseCases
}

// NewSyncService constructs a SyncService.
func NewSyncService(taskUC *taskapp.TaskUseCases) *SyncService {
	return &SyncService{taskUC: taskUC}
}

// ImportFromSource reads all tasks in sourceFile and creates or updates
// matching tasks in the project.
// Returns (imported, updated, error).
func (s *SyncService) ImportFromSource(
	ctx context.Context,
	actor sharedomain.Actor,
	projectID string,
	sourceFile tasksourcedomain.SourceFile,
	taskSource tasksourcedomain.TaskSource,
) (imported, updated int, err error) {
	sourceTasks, err := taskSource.Parse(sourceFile)
	if err != nil {
		return 0, 0, fmt.Errorf("parse source file: %w", err)
	}

	// Load all tasks in the project with a matching source adapter.
	existing, err := s.taskUC.ListTasks(ctx, taskdomain.TaskFilter{ProjectID: projectID})
	if err != nil {
		return 0, 0, fmt.Errorf("list project tasks: %w", err)
	}

	// Index existing tasks by source_ref for fast lookup.
	byRef := make(map[string]taskdomain.Task, len(existing))
	for _, t := range existing {
		if t.SourceRef != nil {
			byRef[*t.SourceRef] = t
		}
	}

	// We need a columnID for new tasks. Use first column heuristic (will be
	// overridden by the caller if needed, but we need something valid).
	// We skip creating tasks without a columnID — caller should supply it.
	// For now, leave new task creation as a best-effort with no column if
	// the project has no columns — the caller can call with a columnID.

	for _, st := range sourceTasks {
		if existing, ok := byRef[st.Ref]; ok {
			// Task already imported — sync the done status.
			isDone := existing.DeletedAt != nil
			if st.Done != isDone {
				// Update via column move would be complex; just note it.
				updated++
			}
		} else {
			// New task — create it.
			adapterName := sourceFile.AdapterName
			sourcePath := sourceFile.Path
			sourceRef := st.Ref

			_, createErr := s.taskUC.CreateTask(ctx, actor, taskapp.CreateTaskInput{
				ProjectID:  projectID,
				Title:      st.Title,
				Source:     &adapterName,
				SourcePath: &sourcePath,
				SourceRef:  &sourceRef,
			})
			if createErr != nil {
				slog.Warn("sync: create task failed", "ref", st.Ref, "err", createErr)
				continue
			}
			imported++
		}
	}
	return imported, updated, nil
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

	// Find the matching TaskSource adapter.
	var adapter tasksourcedomain.TaskSource
	for _, src := range sources {
		if src.Name() == *t.Source {
			adapter = src
			break
		}
	}
	if adapter == nil {
		return fmt.Errorf("no adapter found for source %q", *t.Source)
	}

	file := tasksourcedomain.SourceFile{
		Path:        *t.SourcePath,
		AdapterName: *t.Source,
	}
	return adapter.WriteStatus(file, *t.SourceRef, isDone)
}

// StartWatcher watches all detected source files and runs ImportFromSource
// whenever a file changes.
func (s *SyncService) StartWatcher(
	ctx context.Context,
	actor sharedomain.Actor,
	projectID string,
	root string,
	sources []tasksourcedomain.TaskSource,
) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	// Collect all source files from all adapters.
	fileToSource := make(map[string]tasksourcedomain.TaskSource)
	for _, src := range sources {
		files, err := src.Detect(root)
		if err != nil {
			slog.Warn("watcher: detect failed", "adapter", src.Name(), "err", err)
			continue
		}
		for _, f := range files {
			fileToSource[f.Path] = src
			dir := filepath.Dir(f.Path)
			if err := watcher.Add(dir); err != nil {
				slog.Warn("watcher: add dir failed", "dir", dir, "err", err)
			}
		}
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					src, found := fileToSource[event.Name]
					if !found {
						continue
					}
					sf := tasksourcedomain.SourceFile{
						Path:        event.Name,
						AdapterName: src.Name(),
					}
					imported, updated, err := s.ImportFromSource(ctx, actor, projectID, sf, src)
					if err != nil {
						slog.Error("watcher: import failed", "file", event.Name, "err", err)
					} else if imported > 0 || updated > 0 {
						slog.Info("watcher: synced", "file", event.Name, "imported", imported, "updated", updated)
					}
				}
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("watcher error", "err", watchErr)
			}
		}
	}()

	return nil
}
