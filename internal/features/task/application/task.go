// Package application contains the task feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	sharedomain "github.com/lesquel/termino/internal/domain"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/events"
)

// Actor is a type alias so callers can use the application package directly.
type Actor = sharedomain.Actor

// ErrConfirmRequired is returned by DeleteTask when confirm is false.
var ErrConfirmRequired = errors.New("deletion requires explicit confirmation")

// TaskMoved is published after a task changes columns.
type TaskMoved struct {
	TaskID       string
	FromColumnID string
	ToColumnID   string
}

// Type implements events.Event.
func (e TaskMoved) Type() string { return "task.moved" }

// TaskAssigned is published after a task's assignee changes.
type TaskAssigned struct {
	TaskID     string
	AssigneeID *string
}

// Type implements events.Event.
func (e TaskAssigned) Type() string { return "task.assigned" }

// CreateTaskInput is the input DTO for the CreateTask use case.
type CreateTaskInput struct {
	ProjectID   string
	Title       string
	Description string
	ColumnID    string
	SprintID    *string
	Priority    taskdomain.Priority
	AssigneeID  *string
	DueDate     *time.Time
	Points      *int
	Source      *string
	SourcePath  *string
	SourceRef   *string
}

// UpdateTaskInput is the input DTO for the UpdateTask use case.
type UpdateTaskInput struct {
	TaskID      string
	Title       *string
	Description *string
	Priority    *taskdomain.Priority
	DueDate     *time.Time
	Points      *int
}

// MethodologyResolver returns the workflow strategy of a project, so MoveTask
// can enforce Methodology.CanMove.
type MethodologyResolver interface {
	MethodologyFor(ctx context.Context, projectID string) (projectdomain.Methodology, error)
}

// TaskUseCases groups all task-related use cases.
type TaskUseCases struct {
	tasks         taskdomain.TaskRepository
	comments      taskdomain.CommentRepository
	activities    taskdomain.ActivityRepository
	board         boarddomain.BoardRepository
	bus           *events.Bus
	methodologies MethodologyResolver
	labels        taskdomain.LabelRepository
}

// NewTaskUseCases constructs a TaskUseCases instance.
func NewTaskUseCases(
	tasks taskdomain.TaskRepository,
	comments taskdomain.CommentRepository,
	activities taskdomain.ActivityRepository,
	board boarddomain.BoardRepository,
	bus *events.Bus,
) *TaskUseCases {
	return &TaskUseCases{
		tasks:      tasks,
		comments:   comments,
		activities: activities,
		board:      board,
		bus:        bus,
	}
}

// WithMethodologies makes MoveTask consult the project's methodology before
// every move. Without a resolver only the WIP limit is checked.
func (uc *TaskUseCases) WithMethodologies(r MethodologyResolver) *TaskUseCases {
	uc.methodologies = r
	return uc
}

// WithLabels enables AddLabels, backed by the given label repository.
func (uc *TaskUseCases) WithLabels(r taskdomain.LabelRepository) *TaskUseCases {
	uc.labels = r
	return uc
}

// ErrLabelsUnavailable is returned by AddLabels when no label repository is
// configured.
var ErrLabelsUnavailable = errors.New("labels are not available")

// AddLabels attaches the named labels to a task, creating the ones the
// project does not have yet. Names are matched ignoring case, so an existing
// label keeps its original spelling. Empty names are skipped.
func (uc *TaskUseCases) AddLabels(ctx context.Context, actor Actor, taskID string, names []string) error {
	if uc.labels == nil {
		return ErrLabelsUnavailable
	}
	t, err := uc.tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	var attached []string
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		label, err := uc.labels.FindByName(ctx, t.ProjectID, name)
		if errors.Is(err, taskdomain.ErrLabelNotFound) {
			label = taskdomain.Label{ID: uuid.NewString(), ProjectID: t.ProjectID, Name: name}
			err = uc.labels.Save(ctx, label)
		}
		if err != nil {
			return fmt.Errorf("label %q: %w", name, err)
		}
		if err := uc.labels.Attach(ctx, t.ID, label.ID); err != nil {
			return fmt.Errorf("attach label %q: %w", label.Name, err)
		}
		attached = append(attached, label.Name)
	}
	if len(attached) == 0 {
		return nil
	}
	return uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "labeled", map[string]any{"labels": attached})
}

// CreateTask assigns the next number, sets the fractional position at the end
// of the target column, saves the task, and records a 'created' activity.
func (uc *TaskUseCases) CreateTask(
	ctx context.Context,
	actor Actor,
	input CreateTaskInput,
) (taskdomain.Task, error) {
	// A task without a column would be invisible on the board (and "" is not a
	// valid columns(id) reference), so default to the first column.
	if input.ColumnID == "" {
		cols, err := uc.board.ListColumns(ctx, input.ProjectID)
		if err != nil {
			return taskdomain.Task{}, fmt.Errorf("list columns: %w", err)
		}
		var first boarddomain.Column
		for i, c := range cols {
			if i == 0 || c.Position < first.Position {
				first = c
			}
		}
		input.ColumnID = first.ID
	}
	num, err := uc.tasks.NextNumber(ctx, input.ProjectID)
	if err != nil {
		return taskdomain.Task{}, fmt.Errorf("next number: %w", err)
	}
	existing, err := uc.tasks.List(ctx, taskdomain.TaskFilter{ColumnID: input.ColumnID})
	if err != nil {
		return taskdomain.Task{}, fmt.Errorf("list tasks: %w", err)
	}
	position := float64(len(existing) + 1)

	t := taskdomain.Task{
		ID:          uuid.NewString(),
		ProjectID:   input.ProjectID,
		Number:      num,
		Title:       input.Title,
		Description: input.Description,
		ColumnID:    input.ColumnID,
		SprintID:    input.SprintID,
		Priority:    input.Priority,
		AssigneeID:  input.AssigneeID,
		DueDate:     input.DueDate,
		Points:      input.Points,
		Position:    position,
		Source:      input.Source,
		SourcePath:  input.SourcePath,
		SourceRef:   input.SourceRef,
	}
	if t.Priority == "" {
		t.Priority = taskdomain.PriorityMedium
	}
	if err := uc.tasks.Save(ctx, t); err != nil {
		return taskdomain.Task{}, fmt.Errorf("save task: %w", err)
	}
	if err := uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "created", map[string]any{
		"title": t.Title,
	}); err != nil {
		return taskdomain.Task{}, err
	}
	return t, nil
}

// UpdateTask applies optional field changes and records a 'updated' activity.
func (uc *TaskUseCases) UpdateTask(
	ctx context.Context,
	actor Actor,
	input UpdateTaskInput,
) error {
	t, err := uc.tasks.FindByID(ctx, input.TaskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	changed := map[string]any{}
	if input.Title != nil && *input.Title != t.Title {
		changed["title"] = map[string]any{"from": t.Title, "to": *input.Title}
		t.Title = *input.Title
	}
	if input.Description != nil && *input.Description != t.Description {
		changed["description"] = map[string]any{"from": t.Description, "to": *input.Description}
		t.Description = *input.Description
	}
	if input.Priority != nil && *input.Priority != t.Priority {
		changed["priority"] = map[string]any{"from": t.Priority, "to": *input.Priority}
		t.Priority = *input.Priority
	}
	if input.DueDate != t.DueDate {
		changed["dueDate"] = map[string]any{"from": t.DueDate, "to": input.DueDate}
		t.DueDate = input.DueDate
	}
	if input.Points != t.Points {
		changed["points"] = map[string]any{"from": t.Points, "to": input.Points}
		t.Points = input.Points
	}
	if len(changed) == 0 {
		return nil
	}
	if err := uc.tasks.Save(ctx, t); err != nil {
		return fmt.Errorf("save task: %w", err)
	}
	return uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "updated", changed)
}

// MoveTask relocates a task to a different column at the given fractional position.
// It checks the WIP limit of the target column and records a 'moved' activity.
func (uc *TaskUseCases) MoveTask(
	ctx context.Context,
	actor Actor,
	taskID, targetColumnID string,
	position float64,
) error {
	t, err := uc.tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	col, err := uc.board.FindColumnByID(ctx, targetColumnID)
	if err != nil {
		return fmt.Errorf("find column: %w", err)
	}
	// Build a minimal board to check WIP.
	existingTasks, err := uc.tasks.List(ctx, taskdomain.TaskFilter{ColumnID: targetColumnID})
	if err != nil {
		return fmt.Errorf("list column tasks: %w", err)
	}
	taskIDs := make([]string, 0, len(existingTasks))
	for _, et := range existingTasks {
		if et.ID != taskID { // exclude the task being moved
			taskIDs = append(taskIDs, et.ID)
		}
	}
	board := boarddomain.Board{
		Tasks: map[string][]string{targetColumnID: taskIDs},
	}
	if err := boarddomain.CanAddToColumn(board, col); err != nil {
		return err
	}
	if err := uc.checkMethodology(ctx, t, col); err != nil {
		return err
	}

	fromColumnID := t.ColumnID
	t.ColumnID = targetColumnID
	t.Position = position
	if err := uc.tasks.Save(ctx, t); err != nil {
		return fmt.Errorf("save task: %w", err)
	}
	if err := uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "moved", map[string]any{
		"from": fromColumnID,
		"to":   targetColumnID,
	}); err != nil {
		return err
	}
	uc.bus.Publish(ctx, TaskMoved{TaskID: taskID, FromColumnID: fromColumnID, ToColumnID: targetColumnID})
	return nil
}

// checkMethodology applies the project's Methodology.CanMove rule.
func (uc *TaskUseCases) checkMethodology(ctx context.Context, t taskdomain.Task, to boarddomain.Column) error {
	if uc.methodologies == nil {
		return nil
	}
	m, err := uc.methodologies.MethodologyFor(ctx, t.ProjectID)
	if err != nil {
		return fmt.Errorf("resolve methodology: %w", err)
	}
	cols, err := uc.board.ListColumns(ctx, t.ProjectID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	refs := make([]projectdomain.Column, 0, len(cols))
	for _, c := range cols {
		refs = append(refs, projectColumn(c))
	}
	return m.CanMove(projectdomain.Task{ID: t.ID, ColumnID: t.ColumnID}, projectColumn(to), refs)
}

// projectColumn converts a board column to the project domain's minimal
// reference type used by Methodology.
func projectColumn(c boarddomain.Column) projectdomain.Column {
	return projectdomain.Column{ID: c.ID, Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit}
}

// AssignTask sets or clears the assignee of a task and records an 'assigned' activity.
func (uc *TaskUseCases) AssignTask(
	ctx context.Context,
	actor Actor,
	taskID string,
	assigneeID *string,
) error {
	t, err := uc.tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	t.AssigneeID = assigneeID
	if err := uc.tasks.Save(ctx, t); err != nil {
		return fmt.Errorf("save task: %w", err)
	}
	if err := uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "assigned", map[string]any{
		"assigneeId": assigneeID,
	}); err != nil {
		return err
	}
	uc.bus.Publish(ctx, TaskAssigned{TaskID: taskID, AssigneeID: assigneeID})
	return nil
}

// AddComment attaches a comment to a task.
func (uc *TaskUseCases) AddComment(
	ctx context.Context,
	actor Actor,
	taskID, body string,
) (taskdomain.Comment, error) {
	if _, err := uc.tasks.FindByID(ctx, taskID); err != nil {
		return taskdomain.Comment{}, fmt.Errorf("find task: %w", err)
	}
	c := taskdomain.Comment{
		ID:        uuid.NewString(),
		TaskID:    taskID,
		AuthorID:  actor.ID,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	if err := uc.comments.Save(ctx, c); err != nil {
		return taskdomain.Comment{}, fmt.Errorf("save comment: %w", err)
	}
	return c, nil
}

// DeleteTask soft-deletes a task. Requires confirm==true.
func (uc *TaskUseCases) DeleteTask(
	ctx context.Context,
	actor Actor,
	taskID string,
	confirm bool,
) error {
	if !confirm {
		return ErrConfirmRequired
	}
	t, err := uc.tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	now := time.Now().UTC()
	if err := uc.tasks.SoftDelete(ctx, taskID, now); err != nil {
		return fmt.Errorf("soft delete: %w", err)
	}
	return uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "deleted", map[string]any{})
}

// RestoreTask clears the DeletedAt field on a soft-deleted task.
func (uc *TaskUseCases) RestoreTask(
	ctx context.Context,
	actor Actor,
	taskID string,
) error {
	t, err := uc.tasks.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	if err := uc.tasks.Restore(ctx, taskID); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	return uc.appendActivity(ctx, actor, t.ID, t.ProjectID, "restored", map[string]any{})
}

// ListTasks returns tasks matching the given filter.
func (uc *TaskUseCases) ListTasks(
	ctx context.Context,
	filter taskdomain.TaskFilter,
) ([]taskdomain.Task, error) {
	return uc.tasks.List(ctx, filter)
}

// MyTasks returns all non-deleted tasks assigned to the given actor.
func (uc *TaskUseCases) MyTasks(ctx context.Context, actorID string) ([]taskdomain.Task, error) {
	return uc.tasks.List(ctx, taskdomain.TaskFilter{AssigneeID: actorID})
}

// ListRecentActivity returns the most recent limit Activity entries for a project.
func (uc *TaskUseCases) ListRecentActivity(ctx context.Context, projectID string, limit int) ([]taskdomain.Activity, error) {
	return uc.activities.ListRecent(ctx, projectID, limit)
}

// FindTaskByID retrieves a single task by ID.
func (uc *TaskUseCases) FindTaskByID(ctx context.Context, taskID string) (taskdomain.Task, error) {
	return uc.tasks.FindByID(ctx, taskID)
}

// FindTaskByNumber retrieves a task by its per-project number (the N in TER-N).
func (uc *TaskUseCases) FindTaskByNumber(ctx context.Context, projectID string, number int) (taskdomain.Task, error) {
	return uc.tasks.FindByNumber(ctx, projectID, number)
}

// ListCommentsByTask returns all comments for a task.
func (uc *TaskUseCases) ListCommentsByTask(ctx context.Context, taskID string) ([]taskdomain.Comment, error) {
	return uc.comments.ListByTask(ctx, taskID)
}

// ListActivityByTask returns all activity entries for a task.
func (uc *TaskUseCases) ListActivityByTask(ctx context.Context, taskID string) ([]taskdomain.Activity, error) {
	return uc.activities.ListByTask(ctx, taskID)
}

// appendActivity builds and persists an Activity entry.
func (uc *TaskUseCases) appendActivity(
	ctx context.Context,
	actor Actor,
	taskID, projectID, action string,
	payload map[string]any,
) error {
	// Ensure payload is JSON-round-trippable by marshaling and back.
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal activity payload: %w", err)
	}
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("unmarshal activity payload: %w", err)
	}
	a := taskdomain.Activity{
		ID:        uuid.NewString(),
		TaskID:    taskID,
		ActorID:   actor.ID,
		Action:    action,
		Payload:   p,
		CreatedAt: time.Now().UTC(),
	}
	return uc.activities.Append(ctx, a)
}
