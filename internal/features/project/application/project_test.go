package application_test

import (
	"context"
	"testing"

	sharedomain "github.com/lesquel/termino/internal/domain"
	"github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/platform/events"
)

func testActor() sharedomain.Actor {
	return sharedomain.Actor{ID: "actor-1", Name: "Alice", Kind: sharedomain.ActorKindHuman}
}

func newProjectUC() *application.ProjectUseCases {
	return application.NewProjectUseCases(
		newInMemoryProjectRepo(),
		newInMemoryBoardRepo(),
		events.NewBus(),
	)
}

func TestCreateProject(t *testing.T) {
	tests := []struct {
		name      string
		input     application.CreateProjectInput
		wantErr   bool
		errTarget error
	}{
		{
			name:  "happy path kanban",
			input: application.CreateProjectInput{Key: "WEB", Name: "Web App", Methodology: projectdomain.MethodologyKanban},
		},
		{
			name:  "happy path scrum",
			input: application.CreateProjectInput{Key: "MOB", Name: "Mobile", Methodology: projectdomain.MethodologyScrum},
		},
		{
			name:      "invalid key too short",
			input:     application.CreateProjectInput{Key: "W", Name: "Web"},
			wantErr:   true,
			errTarget: application.ErrInvalidKey,
		},
		{
			name:      "invalid key lowercase",
			input:     application.CreateProjectInput{Key: "web", Name: "Web"},
			wantErr:   true,
			errTarget: application.ErrInvalidKey,
		},
		{
			name:      "invalid key too long",
			input:     application.CreateProjectInput{Key: "TOOLONG", Name: "Too Long"},
			wantErr:   true,
			errTarget: application.ErrInvalidKey,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := newProjectUC()
			p, err := uc.CreateProject(context.Background(), testActor(), tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.ID == "" {
				t.Error("expected non-empty ID")
			}
			if p.Key != tc.input.Key {
				t.Errorf("key = %q, want %q", p.Key, tc.input.Key)
			}
		})
	}
}

func TestCreateProject_DuplicateKey(t *testing.T) {
	uc := newProjectUC()
	input := application.CreateProjectInput{Key: "WEB", Name: "Web App", Methodology: projectdomain.MethodologyKanban}
	if _, err := uc.CreateProject(context.Background(), testActor(), input); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := uc.CreateProject(context.Background(), testActor(), input)
	if err == nil {
		t.Fatal("expected duplicate key error, got nil")
	}
}

func TestCreateProject_DefaultColumnsCreated(t *testing.T) {
	boardRepo := newInMemoryBoardRepo()
	uc := application.NewProjectUseCases(newInMemoryProjectRepo(), boardRepo, events.NewBus())
	input := application.CreateProjectInput{Key: "WEB", Name: "Web", Methodology: projectdomain.MethodologyKanban}
	p, err := uc.CreateProject(context.Background(), testActor(), input)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cols, _ := boardRepo.ListColumns(context.Background(), p.ID)
	if len(cols) == 0 {
		t.Error("expected default columns to be created")
	}
}

func TestCreateProject_PublishesEvent(t *testing.T) {
	bus := events.NewBus()
	uc := application.NewProjectUseCases(newInMemoryProjectRepo(), newInMemoryBoardRepo(), bus)
	var received []events.Event
	bus.Subscribe("project.created", func(e events.Event) { received = append(received, e) })
	_, err := uc.CreateProject(context.Background(), testActor(),
		application.CreateProjectInput{Key: "WEB", Name: "Web", Methodology: projectdomain.MethodologyKanban})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(received) != 1 {
		t.Errorf("event count = %d, want 1", len(received))
	}
}

func TestListProjects(t *testing.T) {
	uc := newProjectUC()
	if _, err := uc.CreateProject(context.Background(), testActor(),
		application.CreateProjectInput{Key: "AA", Name: "A", Methodology: projectdomain.MethodologyKanban}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.CreateProject(context.Background(), testActor(),
		application.CreateProjectInput{Key: "BB", Name: "B", Methodology: projectdomain.MethodologyKanban}); err != nil {
		t.Fatal(err)
	}
	projects, err := uc.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("count = %d, want 2", len(projects))
	}
}

func TestArchiveProject(t *testing.T) {
	uc := newProjectUC()
	p, err := uc.CreateProject(context.Background(), testActor(),
		application.CreateProjectInput{Key: "WEB", Name: "Web", Methodology: projectdomain.MethodologyKanban})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.ArchiveProject(context.Background(), testActor(), p.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	projects, _ := uc.ListProjects(context.Background())
	if len(projects) != 0 {
		t.Errorf("expected archived project excluded from list, got %d", len(projects))
	}
}

func TestChangeMethodology(t *testing.T) {
	uc := newProjectUC()
	p, err := uc.CreateProject(context.Background(), testActor(),
		application.CreateProjectInput{Key: "WEB", Name: "Web", Methodology: projectdomain.MethodologyKanban})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.ChangeMethodology(context.Background(), testActor(), p.ID, projectdomain.MethodologyScrum); err != nil {
		t.Fatalf("change: %v", err)
	}
}

func TestUpdateProjectKeepsIDAndKey(t *testing.T) {
	ctx := context.Background()
	uc := newProjectUC()
	p, err := uc.CreateProject(ctx, testActor(), application.CreateProjectInput{Key: "WEB", Name: "Web", AssociatedPath: "/old"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := uc.UpdateProject(ctx, testActor(), application.UpdateProjectInput{
		ID: p.ID, Name: "Web App", Methodology: projectdomain.MethodologyScrum, AssociatedPath: "/new",
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if got.ID != p.ID || got.Key != "WEB" || got.Name != "Web App" ||
		got.Methodology != projectdomain.MethodologyScrum || got.AssociatedPath != "/new" {
		t.Fatalf("updated project = %+v", got)
	}
	list, _ := uc.ListProjects(ctx)
	if len(list) != 1 {
		t.Fatalf("projects = %d, want 1 (update must not duplicate)", len(list))
	}
	if _, err := uc.UpdateProject(ctx, testActor(), application.UpdateProjectInput{ID: "missing", Name: "x"}); err == nil {
		t.Fatal("updating a missing project should fail")
	}
	if found, err := uc.ProjectByKey(ctx, "WEB"); err != nil || found.ID != p.ID {
		t.Fatalf("ProjectByKey = %+v, %v", found, err)
	}
	if found, err := uc.GetProject(ctx, p.ID); err != nil || found.Name != "Web App" {
		t.Fatalf("GetProject = %+v, %v", found, err)
	}
}
