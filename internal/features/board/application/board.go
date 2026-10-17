// Package application contains the board feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	sharedomain "github.com/lesquel/termino/internal/domain"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// Actor is a type alias so callers can use the application package directly.
type Actor = sharedomain.Actor

// BoardUseCases groups all board-related use cases.
type BoardUseCases struct {
	board boarddomain.BoardRepository
	tasks taskdomain.TaskRepository
}

// NewBoardUseCases constructs a BoardUseCases instance.
func NewBoardUseCases(board boarddomain.BoardRepository, tasks taskdomain.TaskRepository) *BoardUseCases {
	return &BoardUseCases{board: board, tasks: tasks}
}

// GetBoard assembles a Board projection from columns and task IDs.
func (uc *BoardUseCases) GetBoard(ctx context.Context, projectID string) (boarddomain.Board, error) {
	cols, err := uc.board.ListColumns(ctx, projectID)
	if err != nil {
		return boarddomain.Board{}, fmt.Errorf("list columns: %w", err)
	}
	allTasks, err := uc.tasks.List(ctx, taskdomain.TaskFilter{ProjectID: projectID})
	if err != nil {
		return boarddomain.Board{}, fmt.Errorf("list tasks: %w", err)
	}
	tasksByColumn := make(map[string][]string)
	for _, t := range allTasks {
		tasksByColumn[t.ColumnID] = append(tasksByColumn[t.ColumnID], t.ID)
	}
	return boarddomain.Board{
		ProjectID: projectID,
		Columns:   cols,
		Tasks:     tasksByColumn,
	}, nil
}

// GetBoardWithTasks assembles the Board projection together with the full
// tasks of each column, in the same order as Board.Tasks. Presentation layers
// use it to render cards (number, title, assignee) without extra lookups.
func (uc *BoardUseCases) GetBoardWithTasks(
	ctx context.Context,
	projectID string,
) (boarddomain.Board, map[string][]taskdomain.Task, error) {
	cols, err := uc.board.ListColumns(ctx, projectID)
	if err != nil {
		return boarddomain.Board{}, nil, fmt.Errorf("list columns: %w", err)
	}
	allTasks, err := uc.tasks.List(ctx, taskdomain.TaskFilter{ProjectID: projectID})
	if err != nil {
		return boarddomain.Board{}, nil, fmt.Errorf("list tasks: %w", err)
	}
	ids := make(map[string][]string)
	full := make(map[string][]taskdomain.Task)
	for _, t := range allTasks {
		ids[t.ColumnID] = append(ids[t.ColumnID], t.ID)
		full[t.ColumnID] = append(full[t.ColumnID], t)
	}
	return boarddomain.Board{ProjectID: projectID, Columns: cols, Tasks: ids}, full, nil
}

// AddColumn creates a new column on the board.
func (uc *BoardUseCases) AddColumn(
	ctx context.Context,
	actor Actor,
	projectID, name string,
	position int,
) (boarddomain.Column, error) {
	if err := uc.ensureUniqueName(ctx, projectID, "", name); err != nil {
		return boarddomain.Column{}, err
	}
	col := boarddomain.Column{
		ID:        uuid.NewString(),
		ProjectID: projectID,
		Name:      name,
		Position:  position,
	}
	if err := uc.board.SaveColumn(ctx, col); err != nil {
		return boarddomain.Column{}, fmt.Errorf("save column: %w", err)
	}
	return col, nil
}

// RenameColumn updates the name of an existing column.
func (uc *BoardUseCases) RenameColumn(
	ctx context.Context,
	actor Actor,
	columnID, name string,
) error {
	col, err := uc.board.FindColumnByID(ctx, columnID)
	if err != nil {
		return fmt.Errorf("find column: %w", err)
	}
	if err := uc.ensureUniqueName(ctx, col.ProjectID, col.ID, name); err != nil {
		return err
	}
	col.Name = name
	return uc.board.SaveColumn(ctx, col)
}

// ensureUniqueName rejects name when another column of the project (other
// than exceptID) already uses it, ignoring case and surrounding spaces.
func (uc *BoardUseCases) ensureUniqueName(ctx context.Context, projectID, exceptID, name string) error {
	cols, err := uc.board.ListColumns(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	want := strings.TrimSpace(name)
	for _, c := range cols {
		if c.ID != exceptID && strings.EqualFold(strings.TrimSpace(c.Name), want) {
			return fmt.Errorf("%w: a column named %q already exists", ErrDuplicateColumnName, c.Name)
		}
	}
	return nil
}

// ReorderColumns sets new positions for all columns in a project.
func (uc *BoardUseCases) ReorderColumns(
	ctx context.Context,
	actor Actor,
	projectID string,
	ids []string,
) error {
	return uc.board.ReorderColumns(ctx, projectID, ids)
}

// SetWipLimit updates the WIP limit for a column. Pass nil to remove the limit.
func (uc *BoardUseCases) SetWipLimit(
	ctx context.Context,
	actor Actor,
	columnID string,
	limit *int,
) error {
	col, err := uc.board.FindColumnByID(ctx, columnID)
	if err != nil {
		return fmt.Errorf("find column: %w", err)
	}
	col.WIPLimit = limit
	return uc.board.SaveColumn(ctx, col)
}

// DeleteColumn removes a column, refusing if it still contains active tasks.
func (uc *BoardUseCases) DeleteColumn(
	ctx context.Context,
	actor Actor,
	columnID string,
) error {
	col, err := uc.board.FindColumnByID(ctx, columnID)
	if err != nil {
		return fmt.Errorf("find column: %w", err)
	}
	tasks, err := uc.tasks.List(ctx, taskdomain.TaskFilter{ColumnID: col.ID})
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	if len(tasks) > 0 {
		return fmt.Errorf("%w: %d task(s) remain", boarddomain.ErrColumnNotEmpty, len(tasks))
	}
	if delErr := uc.board.DeleteColumn(ctx, columnID); delErr != nil {
		return fmt.Errorf("delete column: %w", delErr)
	}
	return nil
}

// ErrColumnNotFound is re-exported for convenience.
var ErrColumnNotFound = boarddomain.ErrColumnNotFound

// ErrColumnNotEmpty is re-exported for convenience.
var ErrColumnNotEmpty = boarddomain.ErrColumnNotEmpty

// ErrDuplicateColumnName is returned when a column name is already used in
// the project (names are compared ignoring case).
var ErrDuplicateColumnName = errors.New("duplicate column name")

// ErrWIPLimitExceeded is returned when adding a task would breach the column limit.
var ErrWIPLimitExceeded = errors.New("WIP limit exceeded")
