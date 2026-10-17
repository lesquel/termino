// Package infrastructure provides SQLite-backed implementations of the board
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
)

// BoardRepository is the SQLite-backed implementation of
// boarddomain.BoardRepository.
type BoardRepository struct {
	db *sql.DB
}

// NewBoardRepository constructs a BoardRepository backed by db.
func NewBoardRepository(db *sql.DB) *BoardRepository {
	return &BoardRepository{db: db}
}

// SaveColumn inserts or updates a Column row.
func (r *BoardRepository) SaveColumn(ctx context.Context, col boarddomain.Column) error {
	// wip_limit = 0 means "no limit" in the schema (NOT NULL with default 0).
	wipLimit := 0
	if col.WIPLimit != nil {
		wipLimit = *col.WIPLimit
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO columns (id, project_id, name, position, wip_limit)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name      = excluded.name,
			position  = excluded.position,
			wip_limit = excluded.wip_limit`,
		col.ID,
		col.ProjectID,
		col.Name,
		col.Position,
		wipLimit,
	)
	if err != nil {
		return fmt.Errorf("board.SaveColumn: %w", err)
	}
	return nil
}

// FindColumnByID retrieves a Column by its UUID.
func (r *BoardRepository) FindColumnByID(ctx context.Context, id string) (boarddomain.Column, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, position, wip_limit
		FROM   columns
		WHERE  id = ?`, id)
	return scanColumn(row)
}

// ListColumns returns all columns for a project, ordered by position ascending.
func (r *BoardRepository) ListColumns(ctx context.Context, projectID string) ([]boarddomain.Column, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, project_id, name, position, wip_limit
		FROM   columns
		WHERE  project_id = ?
		ORDER  BY position ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("board.ListColumns: %w", err)
	}
	defer rows.Close()
	return collectColumns(rows)
}

// ReorderColumns updates the position of each column identified by ids.
// ids must contain every column ID in the project in the desired order.
func (r *BoardRepository) ReorderColumns(ctx context.Context, projectID string, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("board.ReorderColumns begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			UPDATE columns SET position = ? WHERE id = ? AND project_id = ?`,
			i, id, projectID); err != nil {
			return fmt.Errorf("board.ReorderColumns update %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// DeleteColumn removes a column by ID.
func (r *BoardRepository) DeleteColumn(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM columns WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("board.DeleteColumn: %w", err)
	}
	return nil
}

// scanColumn scans a single column row.
func scanColumn(row *sql.Row) (boarddomain.Column, error) {
	var col boarddomain.Column
	var wipLimit *int
	err := row.Scan(&col.ID, &col.ProjectID, &col.Name, &col.Position, &wipLimit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return boarddomain.Column{}, boarddomain.ErrColumnNotFound
		}
		return boarddomain.Column{}, fmt.Errorf("column scan: %w", err)
	}
	if wipLimit != nil && *wipLimit > 0 {
		col.WIPLimit = wipLimit
	}
	return col, nil
}

// collectColumns reads all column rows from *sql.Rows.
func collectColumns(rows *sql.Rows) ([]boarddomain.Column, error) {
	var cols []boarddomain.Column
	for rows.Next() {
		var col boarddomain.Column
		var wipLimit *int
		if err := rows.Scan(&col.ID, &col.ProjectID, &col.Name, &col.Position, &wipLimit); err != nil {
			return nil, fmt.Errorf("column row scan: %w", err)
		}
		if wipLimit != nil && *wipLimit > 0 {
			col.WIPLimit = wipLimit
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}
