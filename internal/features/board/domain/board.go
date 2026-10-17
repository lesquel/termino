// Package domain defines the board aggregate, column value object, and
// repository interface. No imports outside the Go standard library are used.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Column is a vertical lane on the board into which tasks are placed.
type Column struct {
	ID        string
	ProjectID string
	Name      string
	Position  int
	WIPLimit  *int // nil means no limit
}

// Board is a projection of all columns and the tasks currently in each column
// for a given project. It is not persisted as its own table; it is assembled
// from column and task rows in the application layer.
type Board struct {
	ProjectID string
	Columns   []Column
	// Tasks maps column ID to the list of task IDs in that column (ordered by
	// position). The board intentionally stores minimal task references to
	// avoid circular domain imports.
	Tasks map[string][]string
}

// CanAddToColumn checks whether adding a task to col would violate the WIP
// limit. Returns a non-nil error when the limit would be exceeded.
func CanAddToColumn(board Board, col Column) error {
	if col.WIPLimit == nil {
		return nil
	}
	current := len(board.Tasks[col.ID])
	if current >= *col.WIPLimit {
		return fmt.Errorf("WIP limit of %d reached for column %q", *col.WIPLimit, col.Name)
	}
	return nil
}

// WIPExceeded reports whether col currently holds more tasks than its WIP
// limit allows. A column without a limit never exceeds it.
func WIPExceeded(board Board, col Column) bool {
	return col.WIPLimit != nil && len(board.Tasks[col.ID]) > *col.WIPLimit
}

// DoneColumnName is the conventional name of the column holding finished work.
const DoneColumnName = "Done"

// DoneColumn returns the column that represents completed work: the column
// named "Done" (ignoring case and surrounding spaces) when there is one,
// otherwise the column with the highest position. ok is false when cols is
// empty.
func DoneColumn(cols []Column) (done Column, ok bool) {
	for _, c := range cols {
		if strings.EqualFold(strings.TrimSpace(c.Name), DoneColumnName) {
			return c, true
		}
	}
	for i, c := range cols {
		if i == 0 || c.Position > done.Position {
			done = c
		}
	}
	return done, len(cols) > 0
}

// ErrColumnNotFound is returned when a requested column does not exist.
var ErrColumnNotFound = errors.New("column not found")

// ErrColumnNotEmpty is returned when an attempt is made to delete a column
// that still contains non-deleted tasks.
var ErrColumnNotEmpty = errors.New("column still has tasks")

// BoardRepository defines the persistence contract for board-related data.
type BoardRepository interface {
	// SaveColumn inserts or updates a Column.
	SaveColumn(ctx context.Context, col Column) error
	// FindColumnByID retrieves a Column by its unique identifier.
	FindColumnByID(ctx context.Context, id string) (Column, error)
	// ListColumns returns all columns for a project, ordered by Position.
	ListColumns(ctx context.Context, projectID string) ([]Column, error)
	// ReorderColumns persists the new position values for each column ID.
	// ids must contain every column ID in the project, in the desired order.
	ReorderColumns(ctx context.Context, projectID string, ids []string) error
	// DeleteColumn removes a column by ID. The caller is responsible for
	// ensuring the column is empty before calling this method.
	DeleteColumn(ctx context.Context, id string) error
}
