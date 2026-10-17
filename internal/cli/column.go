package cli

import (
	"context"
	"fmt"
	"strconv"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/spf13/cobra"
)

// newColumnCmd returns `termino column add|list|rename|reorder|delete|wip`.
// Columns are given by name (ignoring case) or ID and belong to the current
// project.
func newColumnCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "column",
		Short: "Manage the columns of the current project's board",
	}
	cmd.AddCommand(
		newColumnAddCmd(),
		newColumnListCmd(),
		newColumnRenameCmd(),
		newColumnReorderCmd(),
		newColumnDeleteCmd(),
		newColumnWipCmd(),
	)
	return cmd
}

func newColumnAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Add a column at the end of the board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			name, err := cleanName("column name", args[0])
			if err != nil {
				return err
			}
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			uc, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			board, err := uc.GetBoard(ctx, projectID)
			if err != nil {
				return fmt.Errorf("get board: %w", err)
			}
			position := 0
			for _, c := range board.Columns {
				if c.Position >= position {
					position = c.Position + 1
				}
			}
			if _, err := uc.AddColumn(ctx, cliActor(), projectID, name, position); err != nil {
				return err
			}
			fmt.Printf("✓ Added column %s\n", name)
			return nil
		},
	}
}

func newColumnListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List columns in board order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			uc, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			board, err := uc.GetBoard(context.Background(), projectID)
			if err != nil {
				return fmt.Errorf("get board: %w", err)
			}
			for _, c := range board.Columns {
				wip := ""
				if c.WIPLimit != nil {
					wip = fmt.Sprintf("WIP %d", *c.WIPLimit)
					if boarddomain.WIPExceeded(board, c) {
						wip += fmt.Sprintf(" (exceeded: %d/%d)", len(board.Tasks[c.ID]), *c.WIPLimit)
					}
				}
				fmt.Printf("  %s\t%d task(s)\t%s\n", sanitize.Line(c.Name), len(board.Tasks[c.ID]), wip)
			}
			return nil
		},
	}
}

func newColumnRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <column> <new-name>",
		Short: "Rename a column",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			name, err := cleanName("column name", args[1])
			if err != nil {
				return err
			}
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			col, _, err := resolveBoardColumn(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			uc, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			if err := uc.RenameColumn(ctx, cliActor(), col.ID, name); err != nil {
				return err
			}
			fmt.Printf("✓ Renamed %s to %s\n", sanitize.Line(col.Name), name)
			return nil
		},
	}
}

func newColumnReorderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reorder <column>...",
		Short: "Set the board order; name every column exactly once",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(args))
			seen := map[string]bool{}
			var all int
			for _, ref := range args {
				col, cols, err := resolveBoardColumn(ctx, projectID, ref)
				if err != nil {
					return err
				}
				all = len(cols)
				if seen[col.ID] {
					return fmt.Errorf("column %s named twice", sanitize.Line(col.Name))
				}
				seen[col.ID] = true
				ids = append(ids, col.ID)
			}
			if len(ids) != all {
				return fmt.Errorf("name all %d columns in the new order (got %d)", all, len(ids))
			}
			uc, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			if err := uc.ReorderColumns(ctx, cliActor(), projectID, ids); err != nil {
				return err
			}
			fmt.Println("✓ Columns reordered")
			return nil
		},
	}
}

func newColumnDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <column>",
		Short: "Delete an empty column",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			col, _, err := resolveBoardColumn(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			uc, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			if err := uc.DeleteColumn(ctx, cliActor(), col.ID); err != nil {
				return err
			}
			fmt.Printf("✓ Deleted column %s\n", sanitize.Line(col.Name))
			return nil
		},
	}
}

func newColumnWipCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wip <column> <limit>",
		Short: "Set a column's WIP limit (0 removes it)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 0 {
				return fmt.Errorf("invalid WIP limit %q (use a whole number, 0 to remove)", sanitize.Line(args[1]))
			}
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			col, _, err := resolveBoardColumn(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			uc, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			var limit *int
			if n > 0 {
				limit = &n
			}
			if err := uc.SetWipLimit(ctx, cliActor(), col.ID, limit); err != nil {
				return err
			}
			if limit == nil {
				fmt.Printf("✓ Removed the WIP limit of %s\n", sanitize.Line(col.Name))
			} else {
				fmt.Printf("✓ WIP limit of %s set to %d\n", sanitize.Line(col.Name), n)
			}
			return nil
		},
	}
}
