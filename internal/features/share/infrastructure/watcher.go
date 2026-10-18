package infrastructure

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/lesquel/termino/internal/features/share/domain"
)

// DebounceInterval is how long the shared directory must stay quiet before a
// watcher imports it: a git pull or checkout touches many files at once.
const DebounceInterval = 300 * time.Millisecond

// Watch watches dir (a share root's .termino directory) and its tasks
// subdirectory, created later if need be, and calls onChange once changes
// have been quiet for debounce. Calls never overlap. It returns once the
// watches are registered; the returned channel closes after ctx is cancelled
// and the watcher has stopped. Watch errors are logged to log (nil discards).
func Watch(ctx context.Context, dir string, debounce time.Duration, onChange func(), log *slog.Logger) (<-chan struct{}, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("share: create watcher: %w", err)
	}
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("share: watch %s: %w", dir, err)
	}
	tasksDir := filepath.Join(dir, domain.TasksDir)
	// A missing tasks directory is added when it is created.
	_ = w.Add(tasksDir)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer w.Close()
		timer := time.NewTimer(debounce)
		timer.Stop()
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Name == tasksDir && ev.Has(fsnotify.Create) {
					if err := w.Add(tasksDir); err != nil {
						log.Warn("share watcher: add tasks dir failed", "dir", tasksDir, "err", err)
					}
				}
				timer.Reset(debounce)
			case <-timer.C:
				onChange()
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Warn("share watcher error", "err", err)
			}
		}
	}()
	return done, nil
}
