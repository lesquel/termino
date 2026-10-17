package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	sharedomain "github.com/lesquel/termino/internal/domain"
	tasksourceapp "github.com/lesquel/termino/internal/features/tasksource/application"
	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
	kirospecadapter "github.com/lesquel/termino/internal/features/tasksource/infrastructure/kirospec"
	markdownadapter "github.com/lesquel/termino/internal/features/tasksource/infrastructure/markdown"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/logging"
	"github.com/spf13/cobra"
)

// newImportCmd returns `termino import [path] [--watch]`.
func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [path]",
		Short: "Import tasks from source files into the current project",
		Long: `Import checkbox tasks from Markdown files (Kiro tasks.md, odd/tasks/*.md or
any "- [ ]" list) as cards in the project's first column. Re-importing never
duplicates cards, and a checked box moves its card to Done.

With --watch, termino keeps running and re-imports the files whenever they
change, until interrupted with Ctrl+C.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runImport,
	}
	cmd.Flags().Bool("watch", false, "keep watching the imported files and sync changes until interrupted")
	return cmd
}

// taskSources returns every registered task source adapter.
func taskSources() []tasksourcedomain.TaskSource {
	return []tasksourcedomain.TaskSource{kirospecadapter.New(), markdownadapter.New()}
}

// fileLogger routes slog output to the termino log file so background sync
// never writes to stdout (TUI, stdio) and returns that logger. When the log
// file cannot be opened, logs are discarded rather than sent to the terminal.
func fileLogger() *slog.Logger {
	if err := logging.Init(filepath.Join(config.StateDir(), "termino.log")); err != nil || logging.Logger == nil {
		discard := slog.New(slog.DiscardHandler)
		slog.SetDefault(discard)
		return discard
	}
	return logging.Logger
}

// newSyncService builds the task-source sync service with a file logger.
func newSyncService() (*tasksourceapp.SyncService, error) {
	taskUC, err := globalContainer.TaskUseCases()
	if err != nil {
		return nil, err
	}
	boardUC, err := globalContainer.BoardUseCases()
	if err != nil {
		return nil, err
	}
	return tasksourceapp.NewSyncService(taskUC, boardUC).WithLogger(fileLogger()), nil
}

func runImport(cmd *cobra.Command, args []string) error {
	cfg, err := globalContainer.Config()
	if err != nil {
		return err
	}
	if cfg.CurrentProjectID == "" {
		return fmt.Errorf("no current project set — run `termino project new` first")
	}
	watch, _ := cmd.Flags().GetBool("watch")

	svc, err := newSyncService()
	if err != nil {
		return err
	}

	ctx := context.Background()
	actor := sharedomain.Actor{ID: "cli", Name: cfg.UserName, Kind: "human"}
	sources := taskSources()

	var files []tasksourcedomain.SourceFile
	if len(args) == 1 {
		// Explicit path → use the generic markdown adapter.
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve %s: %w", args[0], err)
		}
		files = append(files, tasksourcedomain.SourceFile{Path: abs, AdapterName: markdownadapter.New().Name()})
	} else {
		// Auto-detect in $PWD using all registered adapters.
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		files = detectSourceFiles(root, sources)
	}

	var totalImported, totalUpdated int
	for _, f := range files {
		imported, updated, err := svc.ImportFromSource(ctx, actor, cfg.CurrentProjectID, f, sourceByName(sources, f.AdapterName))
		if err != nil {
			if len(args) == 1 {
				return fmt.Errorf("import from %s: %w", f.Path, err)
			}
			fmt.Fprintf(os.Stderr, "warning: import %s: %v\n", f.Path, err)
			continue
		}
		totalImported += imported
		totalUpdated += updated
	}
	fmt.Printf("Import complete: %d imported, %d updated\n", totalImported, totalUpdated)

	if !watch {
		return nil
	}
	if len(files) == 0 {
		return fmt.Errorf("nothing to watch: no task source files found")
	}
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	done, err := svc.WatchFiles(watchCtx, actor, cfg.CurrentProjectID, files, sources)
	if err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}
	fmt.Printf("Watching %d file(s) for changes — press Ctrl+C to stop\n", len(files))
	<-done
	fmt.Println("Watcher stopped")
	return nil
}

// detectSourceFiles runs every adapter's Detect under root. A file claimed by
// an earlier (more specific) adapter is not claimed again by a later one.
func detectSourceFiles(root string, sources []tasksourcedomain.TaskSource) []tasksourcedomain.SourceFile {
	seen := make(map[string]bool)
	var files []tasksourcedomain.SourceFile
	for _, adapter := range sources {
		detected, err := adapter.Detect(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s detect: %v\n", adapter.Name(), err)
			continue
		}
		for _, f := range detected {
			if abs, err := filepath.Abs(f.Path); err == nil {
				f.Path = abs
			}
			if seen[f.Path] {
				continue
			}
			seen[f.Path] = true
			files = append(files, f)
		}
	}
	return files
}

func sourceByName(sources []tasksourcedomain.TaskSource, name string) tasksourcedomain.TaskSource {
	for _, s := range sources {
		if s.Name() == name {
			return s
		}
	}
	return markdownadapter.New()
}
