package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"

	sharedomain "github.com/lesquel/termino/internal/features/share/domain"
	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
)

// noShareSync lists the top-level commands that never import shared files:
// they do not read the board (version, theme, completion, help), must stay
// fast (the shell widget runs on every prompt), or manage the installation
// itself (setup, uninstall, doctor).
var noShareSync = map[string]bool{
	"version": true, "theme": true, "completion": true, "help": true,
	"widget": true, "setup": true, "uninstall": true, "doctor": true,
	"__complete": true, "__completeNoDesc": true,
}

// topLevel returns the name of the root's child that cmd belongs to; the
// root itself (the TUI) yields "".
func topLevel(cmd *cobra.Command) string {
	for cmd.HasParent() && cmd.Parent().HasParent() {
		cmd = cmd.Parent()
	}
	if !cmd.HasParent() {
		return ""
	}
	return cmd.Name()
}

// syncSharedOnStart imports the current project's shared files before a
// command runs, so every command (read-only ones and `mcp --read-only`
// included: import writes only the local database) sees what teammates
// pushed. Failures are logged, never printed: stdout may be the MCP channel.
func syncSharedOnStart(cmd *cobra.Command) {
	if noShareSync[topLevel(cmd)] {
		return
	}
	projectID, err := currentProjectID()
	if err != nil {
		return
	}
	importShared(context.Background(), projectID)
}

// importShared imports a shared project, logging the outcome. A project
// without a share root is left alone.
func importShared(ctx context.Context, projectID string) {
	svc, err := globalContainer.ShareService()
	if err != nil {
		fileLogger().Warn("share import: service unavailable", "err", err)
		return
	}
	res, err := svc.Import(ctx, projectID)
	if err != nil {
		fileLogger().Warn("share import failed", "project", projectID, "err", err)
	}
	for _, s := range res.Skipped {
		fileLogger().Warn("share import: file skipped", "project", projectID, "path", s.Path, "reason", s.Reason)
	}
}

// shareWatch imports a shared project whenever its .termino directory
// changes while the TUI or the MCP server runs. Version increases after
// every import so the board knows to reload (its data_version poll does not
// see writes made on its own connection), and Warning names the files the
// last import skipped.
type shareWatch struct {
	version atomic.Int64
	mu      sync.Mutex
	warning string
}

// Version returns a counter that increases after every watched import.
func (w *shareWatch) Version() int64 { return w.version.Load() }

// Warning returns the board status warning for skipped files, or "".
func (w *shareWatch) Warning() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.warning
}

func (w *shareWatch) refresh(ctx context.Context, projectID string) {
	svc, err := globalContainer.ShareService()
	if err != nil {
		return
	}
	st, err := svc.Status(ctx, projectID)
	if err != nil {
		return
	}
	w.mu.Lock()
	w.warning = skippedWarning(st.Skipped)
	w.mu.Unlock()
}

// skippedWarning formats the board warning for skipped shared files.
func skippedWarning(skipped []sharedomain.Skipped) string {
	switch len(skipped) {
	case 0:
		return ""
	case 1:
		return "⚠ 1 shared file skipped — termino share status"
	default:
		return fmt.Sprintf("⚠ %d shared files skipped — termino share status", len(skipped))
	}
}

// watchShared starts watching projectID's shared directory. The returned
// channel closes once ctx is cancelled and the watcher stopped; it is
// already closed when the project is not shared or cannot be watched.
func watchShared(ctx context.Context, projectID string) (*shareWatch, <-chan struct{}) {
	w := &shareWatch{}
	done := closedChan()
	if projectID == "" {
		return w, done
	}
	projectUC, err := globalContainer.ProjectUseCases()
	if err != nil {
		return w, done
	}
	p, err := projectUC.GetProject(ctx, projectID)
	if err != nil || p.ShareRoot == "" {
		return w, done
	}
	w.refresh(ctx, projectID)
	dir := filepath.Join(p.ShareRoot, sharedomain.Dir)
	ch, err := shareinfra.Watch(ctx, dir, shareinfra.DebounceInterval, func() {
		importShared(ctx, projectID)
		w.refresh(ctx, projectID)
		w.version.Add(1)
	}, fileLogger())
	if err != nil {
		fileLogger().Warn("share watcher not started", "dir", dir, "err", err)
		return w, done
	}
	return w, ch
}
