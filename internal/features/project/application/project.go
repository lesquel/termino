// Package application contains the project feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	sharedomain "github.com/lesquel/termino/internal/domain"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/platform/events"
)

// Actor is a type alias so callers can use the application package directly.
type Actor = sharedomain.Actor

// ErrDuplicateKey is returned when a project with the same key already exists.
var ErrDuplicateKey = errors.New("project key already in use")

// ErrInvalidKey is returned when the project key does not match the required format.
var ErrInvalidKey = errors.New("project key must be 2-5 uppercase letters")

var keyRegexp = regexp.MustCompile(`^[A-Z]{2,5}$`)

// ProjectCreated is published after a project is created.
type ProjectCreated struct {
	ProjectID string
	Key       string
}

// Type implements events.Event.
func (e ProjectCreated) Type() string { return "project.created" }

// CreateProjectInput is the input DTO for the CreateProject use case.
type CreateProjectInput struct {
	Key            string
	Name           string
	Methodology    projectdomain.MethodologyKind
	AssociatedPath string // optional CWD the widget uses to auto-detect this project
}

// ProjectUseCases groups all project-related use cases.
type ProjectUseCases struct {
	projects projectdomain.ProjectRepository
	board    boarddomain.BoardRepository
	bus      *events.Bus
}

// NewProjectUseCases constructs a ProjectUseCases instance.
func NewProjectUseCases(
	projects projectdomain.ProjectRepository,
	board boarddomain.BoardRepository,
	bus *events.Bus,
) *ProjectUseCases {
	return &ProjectUseCases{projects: projects, board: board, bus: bus}
}

// CreateProject validates the input, generates a UUID, saves the project, creates
// default columns from the methodology, and publishes ProjectCreated.
func (uc *ProjectUseCases) CreateProject(
	ctx context.Context,
	actor Actor,
	input CreateProjectInput,
) (projectdomain.Project, error) {
	if !keyRegexp.MatchString(input.Key) {
		return projectdomain.Project{}, ErrInvalidKey
	}
	if _, err := uc.projects.FindByKey(ctx, input.Key); err == nil {
		return projectdomain.Project{}, fmt.Errorf("%w: %s", ErrDuplicateKey, input.Key)
	}
	if input.Methodology == "" {
		input.Methodology = projectdomain.MethodologyKanban
	}

	p := projectdomain.Project{
		ID:             uuid.NewString(),
		Key:            input.Key,
		Name:           input.Name,
		Methodology:    input.Methodology,
		AssociatedPath: input.AssociatedPath,
		CreatedAt:      time.Now().UTC(),
	}
	if err := uc.projects.Save(ctx, p); err != nil {
		return projectdomain.Project{}, fmt.Errorf("save project: %w", err)
	}

	method := projectdomain.MethodologyFor(input.Methodology)
	for _, tpl := range method.DefaultColumns() {
		col := boarddomain.Column{
			ID:        uuid.NewString(),
			ProjectID: p.ID,
			Name:      tpl.Name,
			Position:  tpl.Position,
		}
		if err := uc.board.SaveColumn(ctx, col); err != nil {
			return projectdomain.Project{}, fmt.Errorf("save column %q: %w", tpl.Name, err)
		}
	}

	uc.bus.Publish(ctx, ProjectCreated{ProjectID: p.ID, Key: p.Key})
	return p, nil
}

// ListProjects returns all non-archived projects.
func (uc *ProjectUseCases) ListProjects(ctx context.Context) ([]projectdomain.Project, error) {
	return uc.projects.List(ctx)
}

// ChangeMethodology updates the methodology of a project.
func (uc *ProjectUseCases) ChangeMethodology(
	ctx context.Context,
	actor Actor,
	projectID string,
	kind projectdomain.MethodologyKind,
) error {
	p, err := uc.projects.FindByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	p.Methodology = kind
	return uc.projects.Save(ctx, p)
}

// UpdateProjectInput is the input DTO for UpdateProject. The key is not part
// of it: task IDs (KEY-N) are derived from it, so it never changes.
type UpdateProjectInput struct {
	ID             string
	Name           string
	Methodology    projectdomain.MethodologyKind // "" keeps the current one
	AssociatedPath string                        // "" removes the association
}

// UpdateProject changes an existing project's name, methodology and
// associated path in place, keeping its ID, key and columns.
func (uc *ProjectUseCases) UpdateProject(
	ctx context.Context,
	actor Actor,
	input UpdateProjectInput,
) (projectdomain.Project, error) {
	p, err := uc.projects.FindByID(ctx, input.ID)
	if err != nil {
		return projectdomain.Project{}, fmt.Errorf("find project: %w", err)
	}
	p.Name = input.Name
	if input.Methodology != "" {
		p.Methodology = input.Methodology
	}
	p.AssociatedPath = input.AssociatedPath
	if err := uc.projects.Save(ctx, p); err != nil {
		return projectdomain.Project{}, fmt.Errorf("save project: %w", err)
	}
	return p, nil
}

// GetProject returns the non-archived project with the given ID.
func (uc *ProjectUseCases) GetProject(ctx context.Context, id string) (projectdomain.Project, error) {
	return uc.projects.FindByID(ctx, id)
}

// ProjectByKey returns the non-archived project with the given key.
func (uc *ProjectUseCases) ProjectByKey(ctx context.Context, key string) (projectdomain.Project, error) {
	return uc.projects.FindByKey(ctx, key)
}

// ArchiveProject marks a project as archived.
func (uc *ProjectUseCases) ArchiveProject(
	ctx context.Context,
	actor Actor,
	projectID string,
) error {
	return uc.projects.Archive(ctx, projectID, time.Now().UTC())
}

// MethodologyFor returns the workflow strategy of the given project. The task
// feature uses it to enforce Methodology.CanMove on every move.
func (uc *ProjectUseCases) MethodologyFor(ctx context.Context, projectID string) (projectdomain.Methodology, error) {
	p, err := uc.projects.FindByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("find project: %w", err)
	}
	return projectdomain.MethodologyFor(p.Methodology), nil
}
