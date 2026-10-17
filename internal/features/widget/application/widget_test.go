package application_test

import (
	"context"
	"errors"
	"testing"

	widgetapp "github.com/lesquel/termino/internal/features/widget/application"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// stubRepo records the project ID it was asked for.
type stubRepo struct{ gotProject string }

func (r *stubRepo) GetSummary(_ context.Context, projectID, _ string) (widgetdomain.Summary, error) {
	r.gotProject = projectID
	return widgetdomain.Summary{ProjectKey: projectID}, nil
}

// stubDetector maps a path to a project ID ("" when unknown).
type stubDetector struct{ byPath map[string]string }

func (d stubDetector) DetectForPath(p string) (string, error) { return d.byPath[p], nil }

func TestGetSummary_CwdDetectionWinsOverFallback(t *testing.T) {
	repo := &stubRepo{}
	uc := widgetapp.NewWidgetUseCases(repo, stubDetector{byPath: map[string]string{"/work/web": "web"}})

	if _, err := uc.GetSummary(context.Background(), "current", "cli", "/work/web"); err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if repo.gotProject != "web" {
		t.Fatalf("project = %q, want detected %q", repo.gotProject, "web")
	}
}

func TestGetSummary_FallsBackToCurrentProject(t *testing.T) {
	repo := &stubRepo{}
	uc := widgetapp.NewWidgetUseCases(repo, stubDetector{byPath: map[string]string{}})

	if _, err := uc.GetSummary(context.Background(), "current", "cli", "/elsewhere"); err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if repo.gotProject != "current" {
		t.Fatalf("project = %q, want fallback %q", repo.gotProject, "current")
	}
}

func TestGetSummary_ErrorsWhenNeitherDetectedNorCurrent(t *testing.T) {
	uc := widgetapp.NewWidgetUseCases(&stubRepo{}, stubDetector{byPath: map[string]string{}})

	_, err := uc.GetSummary(context.Background(), "", "cli", "/elsewhere")
	if !errors.Is(err, widgetapp.ErrNoProject) {
		t.Fatalf("err = %v, want ErrNoProject", err)
	}
}
