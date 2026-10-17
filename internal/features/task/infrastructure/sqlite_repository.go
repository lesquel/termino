// Package infrastructure provides SQLite-backed implementations of the task
// domain repository interfaces.
package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// TaskRepository is the SQLite-backed implementation of taskdomain.TaskRepository.
type TaskRepository struct {
	db *sql.DB
}

// NewTaskRepository constructs a TaskRepository backed by db.
func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// Save inserts or updates a Task row.
func (r *TaskRepository) Save(ctx context.Context, t taskdomain.Task) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var deletedAt *string
	if t.DeletedAt != nil {
		s := t.DeletedAt.UTC().Format(time.RFC3339)
		deletedAt = &s
	}
	var dueDate *string
	if t.DueDate != nil {
		s := t.DueDate.UTC().Format(time.RFC3339)
		dueDate = &s
	}
	var sprintID *string
	if t.SprintID != nil {
		sprintID = t.SprintID
	}
	var assigneeID *string
	if t.AssigneeID != nil {
		assigneeID = t.AssigneeID
	}
	var points int
	if t.Points != nil {
		points = *t.Points
	}
	source := ""
	if t.Source != nil {
		source = *t.Source
	}
	sourcePath := ""
	if t.SourcePath != nil {
		sourcePath = *t.SourcePath
	}
	sourceRef := ""
	if t.SourceRef != nil {
		sourceRef = *t.SourceRef
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tasks
			(id, project_id, number, title, description, column_id, sprint_id,
			 priority, assignee_id, due_date, points, position, deleted_at,
			 source, source_path, source_ref, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title       = excluded.title,
			description = excluded.description,
			column_id   = excluded.column_id,
			sprint_id   = excluded.sprint_id,
			priority    = excluded.priority,
			assignee_id = excluded.assignee_id,
			due_date    = excluded.due_date,
			points      = excluded.points,
			position    = excluded.position,
			deleted_at  = excluded.deleted_at,
			source      = excluded.source,
			source_path = excluded.source_path,
			source_ref  = excluded.source_ref,
			updated_at  = excluded.updated_at`,
		t.ID, t.ProjectID, t.Number, t.Title, t.Description,
		t.ColumnID, sprintID, string(t.Priority), assigneeID,
		dueDate, points, t.Position, deletedAt,
		source, sourcePath, sourceRef, now, now,
	)
	if err != nil {
		return fmt.Errorf("task.Save: %w", err)
	}
	return nil
}

// FindByID retrieves a Task by its UUID (including soft-deleted).
func (r *TaskRepository) FindByID(ctx context.Context, id string) (taskdomain.Task, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, number, title, description, column_id, sprint_id,
		       priority, assignee_id, due_date, points, position, deleted_at,
		       source, source_path, source_ref, `+labelsColumn+`
		FROM   tasks
		WHERE  id = ?`, id)
	return scanTask(row)
}

// FindByNumber retrieves a non-deleted Task by its project-scoped number.
func (r *TaskRepository) FindByNumber(ctx context.Context, projectID string, number int) (taskdomain.Task, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, number, title, description, column_id, sprint_id,
		       priority, assignee_id, due_date, points, position, deleted_at,
		       source, source_path, source_ref, `+labelsColumn+`
		FROM   tasks
		WHERE  project_id = ? AND number = ? AND deleted_at IS NULL`, projectID, number)
	return scanTask(row)
}

// List returns tasks matching the supplied filter.
func (r *TaskRepository) List(ctx context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	var conditions []string
	var args []any

	if filter.ProjectID != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.ColumnID != "" {
		conditions = append(conditions, "column_id = ?")
		args = append(args, filter.ColumnID)
	}
	if filter.AssigneeID != "" {
		conditions = append(conditions, "assignee_id = ?")
		args = append(args, filter.AssigneeID)
	}
	if filter.SprintID != "" {
		conditions = append(conditions, "sprint_id = ?")
		args = append(args, filter.SprintID)
	}
	if filter.SearchText != "" {
		conditions = append(conditions, "(title LIKE ? OR description LIKE ?)")
		like := "%" + filter.SearchText + "%"
		args = append(args, like, like)
	}
	if len(filter.LabelIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(filter.LabelIDs)), ",")
		conditions = append(conditions, "EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = tasks.id AND tl.label_id IN ("+placeholders+"))")
		for _, id := range filter.LabelIDs {
			args = append(args, id)
		}
	}
	if !filter.IncludeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}

	query := `
		SELECT id, project_id, number, title, description, column_id, sprint_id,
		       priority, assignee_id, due_date, points, position, deleted_at,
		       source, source_path, source_ref, ` + labelsColumn + `
		FROM   tasks`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY position ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("task.List: %w", err)
	}
	defer rows.Close()
	return collectTasks(rows)
}

// SoftDelete sets DeletedAt on the task.
func (r *TaskRepository) SoftDelete(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// Restore clears DeletedAt on a soft-deleted task.
func (r *TaskRepository) Restore(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = NULL, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// NextNumber returns the next available task number for the project.
func (r *TaskRepository) NextNumber(ctx context.Context, projectID string) (int, error) {
	var max int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) FROM tasks WHERE project_id = ?`, projectID).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("task.NextNumber: %w", err)
	}
	return max + 1, nil
}

// scanTask scans a single task row from a *sql.Row.
// labelSeparator joins label names in the labels projection; the ASCII unit
// separator cannot appear in a sanitized label name.
const labelSeparator = "\x1f"

// labelsColumn projects the names of a task's labels as one joined string.
const labelsColumn = `COALESCE((SELECT group_concat(l.name, char(31))
		       FROM task_labels tl JOIN labels l ON l.id = tl.label_id
		       WHERE tl.task_id = tasks.id), '') AS labels`

// splitLabels turns the joined labels projection into sorted label names.
func splitLabels(joined string) []string {
	if joined == "" {
		return nil
	}
	names := strings.Split(joined, labelSeparator)
	sort.Strings(names)
	return names
}

func scanTask(row *sql.Row) (taskdomain.Task, error) {
	var t taskdomain.Task
	var deletedAt, dueDate *string
	var sprintID, assigneeID *string
	var points int
	var source, sourcePath, sourceRef, labels string
	var priority string

	err := row.Scan(
		&t.ID, &t.ProjectID, &t.Number, &t.Title, &t.Description,
		&t.ColumnID, &sprintID, &priority, &assigneeID,
		&dueDate, &points, &t.Position, &deletedAt,
		&source, &sourcePath, &sourceRef, &labels,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return taskdomain.Task{}, taskdomain.ErrTaskNotFound
		}
		return taskdomain.Task{}, fmt.Errorf("task scan: %w", err)
	}

	t.Priority = taskdomain.Priority(priority)
	t.Labels = splitLabels(labels)
	t.SprintID = sprintID
	t.AssigneeID = assigneeID
	if points > 0 {
		t.Points = &points
	}
	if source != "" {
		t.Source = &source
	}
	if sourcePath != "" {
		t.SourcePath = &sourcePath
	}
	if sourceRef != "" {
		t.SourceRef = &sourceRef
	}
	if dueDate != nil {
		dd, err := time.Parse(time.RFC3339, *dueDate)
		if err != nil {
			return taskdomain.Task{}, fmt.Errorf("task parse due_date: %w", err)
		}
		t.DueDate = &dd
	}
	if deletedAt != nil {
		da, err := time.Parse(time.RFC3339, *deletedAt)
		if err != nil {
			return taskdomain.Task{}, fmt.Errorf("task parse deleted_at: %w", err)
		}
		t.DeletedAt = &da
	}
	return t, nil
}

// collectTasks reads all task rows from *sql.Rows.
func collectTasks(rows *sql.Rows) ([]taskdomain.Task, error) {
	var tasks []taskdomain.Task
	for rows.Next() {
		var t taskdomain.Task
		var deletedAt, dueDate *string
		var sprintID, assigneeID *string
		var points int
		var source, sourcePath, sourceRef, labels string
		var priority string

		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.Number, &t.Title, &t.Description,
			&t.ColumnID, &sprintID, &priority, &assigneeID,
			&dueDate, &points, &t.Position, &deletedAt,
			&source, &sourcePath, &sourceRef, &labels,
		); err != nil {
			return nil, fmt.Errorf("task row scan: %w", err)
		}
		t.Priority = taskdomain.Priority(priority)
		t.Labels = splitLabels(labels)
		t.SprintID = sprintID
		t.AssigneeID = assigneeID
		if points > 0 {
			t.Points = &points
		}
		if source != "" {
			t.Source = &source
		}
		if sourcePath != "" {
			t.SourcePath = &sourcePath
		}
		if sourceRef != "" {
			t.SourceRef = &sourceRef
		}
		if dueDate != nil {
			dd, err := time.Parse(time.RFC3339, *dueDate)
			if err != nil {
				return nil, fmt.Errorf("task parse due_date: %w", err)
			}
			t.DueDate = &dd
		}
		if deletedAt != nil {
			da, err := time.Parse(time.RFC3339, *deletedAt)
			if err != nil {
				return nil, fmt.Errorf("task parse deleted_at: %w", err)
			}
			t.DeletedAt = &da
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// CommentRepository is the SQLite-backed implementation of taskdomain.CommentRepository.
type CommentRepository struct {
	db *sql.DB
}

// NewCommentRepository constructs a CommentRepository backed by db.
func NewCommentRepository(db *sql.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

// Save inserts or updates a Comment row.
func (r *CommentRepository) Save(ctx context.Context, c taskdomain.Comment) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO comments (id, task_id, author_id, body, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			body = excluded.body`,
		c.ID, c.TaskID, c.AuthorID, c.Body,
		c.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("comment.Save: %w", err)
	}
	return nil
}

// ListByTask returns all comments for a task, ordered by created_at ascending.
func (r *CommentRepository) ListByTask(ctx context.Context, taskID string) ([]taskdomain.Comment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, author_id, body, created_at
		FROM   comments
		WHERE  task_id = ?
		ORDER  BY created_at ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("comment.ListByTask: %w", err)
	}
	defer rows.Close()

	var comments []taskdomain.Comment
	for rows.Next() {
		var c taskdomain.Comment
		var createdAt string
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.Body, &createdAt); err != nil {
			return nil, fmt.Errorf("comment row scan: %w", err)
		}
		var parseErr error
		c.CreatedAt, parseErr = time.Parse(time.RFC3339, createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("comment parse created_at: %w", parseErr)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// ActivityRepository is the SQLite-backed implementation of taskdomain.ActivityRepository.
type ActivityRepository struct {
	db *sql.DB
}

// NewActivityRepository constructs an ActivityRepository backed by db.
func NewActivityRepository(db *sql.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

// Append inserts a new Activity entry. Never updates or deletes.
func (r *ActivityRepository) Append(ctx context.Context, a taskdomain.Activity) error {
	payload, err := json.Marshal(a.Payload)
	if err != nil {
		return fmt.Errorf("activity.Append marshal: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO activity (id, task_id, actor_id, action, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.TaskID, a.ActorID, a.Action,
		string(payload),
		a.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("activity.Append: %w", err)
	}
	return nil
}

// ListByTask returns all activity entries for a task, oldest first.
func (r *ActivityRepository) ListByTask(ctx context.Context, taskID string) ([]taskdomain.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, actor_id, action, payload, created_at
		FROM   activity
		WHERE  task_id = ?
		ORDER  BY created_at ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("activity.ListByTask: %w", err)
	}
	defer rows.Close()
	return collectActivities(rows)
}

// ListRecent returns the most recent limit entries across a project.
func (r *ActivityRepository) ListRecent(ctx context.Context, projectID string, limit int) ([]taskdomain.Activity, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.task_id, a.actor_id, a.action, a.payload, a.created_at
		FROM   activity a
		JOIN   tasks t ON t.id = a.task_id
		WHERE  t.project_id = ?
		ORDER  BY a.created_at DESC
		LIMIT  ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("activity.ListRecent: %w", err)
	}
	defer rows.Close()
	return collectActivities(rows)
}

// collectActivities reads activity rows from *sql.Rows.
func collectActivities(rows *sql.Rows) ([]taskdomain.Activity, error) {
	var activities []taskdomain.Activity
	for rows.Next() {
		var a taskdomain.Activity
		var createdAt, payloadStr string
		if err := rows.Scan(&a.ID, &a.TaskID, &a.ActorID, &a.Action, &payloadStr, &createdAt); err != nil {
			return nil, fmt.Errorf("activity row scan: %w", err)
		}
		if err := json.Unmarshal([]byte(payloadStr), &a.Payload); err != nil {
			return nil, fmt.Errorf("activity unmarshal payload: %w", err)
		}
		var err error
		a.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("activity parse created_at: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// LabelRepository is the SQLite-backed implementation of taskdomain.LabelRepository.
type LabelRepository struct {
	db *sql.DB
}

// NewLabelRepository constructs a LabelRepository backed by db.
func NewLabelRepository(db *sql.DB) *LabelRepository {
	return &LabelRepository{db: db}
}

// FindByName returns the project's label named name, ignoring case.
func (r *LabelRepository) FindByName(ctx context.Context, projectID, name string) (taskdomain.Label, error) {
	var l taskdomain.Label
	err := r.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, color
		FROM   labels
		WHERE  project_id = ? AND lower(name) = lower(?)
		ORDER  BY rowid
		LIMIT  1`, projectID, name).Scan(&l.ID, &l.ProjectID, &l.Name, &l.Color)
	if errors.Is(err, sql.ErrNoRows) {
		return taskdomain.Label{}, taskdomain.ErrLabelNotFound
	}
	if err != nil {
		return taskdomain.Label{}, fmt.Errorf("label.FindByName: %w", err)
	}
	return l, nil
}

// Save inserts a new label.
func (r *LabelRepository) Save(ctx context.Context, l taskdomain.Label) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO labels (id, project_id, name, color) VALUES (?, ?, ?, ?)`,
		l.ID, l.ProjectID, l.Name, l.Color)
	if err != nil {
		return fmt.Errorf("label.Save: %w", err)
	}
	return nil
}

// Attach links a label to a task, ignoring a link that already exists.
func (r *LabelRepository) Attach(ctx context.Context, taskID, labelID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO task_labels (task_id, label_id) VALUES (?, ?)`, taskID, labelID)
	if err != nil {
		return fmt.Errorf("label.Attach: %w", err)
	}
	return nil
}
