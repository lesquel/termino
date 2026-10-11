package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	sharedomain "github.com/lesquel/termino/internal/domain"
	tasksourceapp "github.com/lesquel/termino/internal/features/tasksource/application"
	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
	kirospecadapter "github.com/lesquel/termino/internal/features/tasksource/infrastructure/kirospec"
	markdownadapter "github.com/lesquel/termino/internal/features/tasksource/infrastructure/markdown"
)

// newImportCmd returns `termino import [path]`.
func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import [path]",
		Short: "Import tasks from source files into the current project",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runImport,
	}
}

func runImport(cmd *cobra.Command, args []string) error {
	cfg, err := globalContainer.Config()
	if err != nil {
		return err
	}
	if cfg.CurrentProjectID == "" {
		return fmt.Errorf("no current project set — run `termino project new` first")
	}

	taskUC, err := globalContainer.TaskUseCases()
	if err != nil {
		return err
	}

	ctx := context.Background()
	actor := sharedomain.Actor{ID: "cli", Name: cfg.UserName, Kind: "human"}
	svc := tasksourceapp.NewSyncService(taskUC)

	kiroSpec := kirospecadapter.New()
	markdown := markdownadapter.New()

	var totalImported, totalUpdated int

	if len(args) == 1 {
		// Explicit path → use markdown adapter.
		sf := tasksourcedomain.SourceFile{Path: args[0], AdapterName: markdown.Name()}
		imported, updated, err := svc.ImportFromSource(ctx, actor, cfg.CurrentProjectID, sf, markdown)
		if err != nil {
			return fmt.Errorf("import from %s: %w", args[0], err)
		}
		totalImported += imported
		totalUpdated += updated
	} else {
		// Auto-detect in $PWD using all registered adapters.
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}

		adapters := []tasksourcedomain.TaskSource{kiroSpec, markdown}
		for _, adapter := range adapters {
			files, err := adapter.Detect(root)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %s detect: %v\n", adapter.Name(), err)
				continue
			}
			for _, f := range files {
				imported, updated, err := svc.ImportFromSource(ctx, actor, cfg.CurrentProjectID, f, adapter)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: import %s: %v\n", f.Path, err)
					continue
				}
				totalImported += imported
				totalUpdated += updated
			}
		}
	}

	fmt.Printf("Import complete: %d imported, %d updated\n", totalImported, totalUpdated)
	return nil
}
