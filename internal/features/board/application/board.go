// Package application contains the board feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"errors"
	"fmt"

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

// AddColumn creates a new column on the board.
func (uc *BoardUseCases) AddColumn(
	ctx context.Context,
	actor Actor,
	projectID, name string,
	position int,
) (boarddomain.Column, error) {
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
	col.Name = name
	return uc.board.SaveColumn(ctx, col)
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

// ErrWIPLimitExceeded is returned when adding a task would breach the column limit.
var ErrWIPLimitExceeded = errors.New("WIP limit exceeded")
