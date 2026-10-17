// Package application contains the widget feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"errors"
	"fmt"

	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// ErrNoProject is returned when no project can be resolved for the widget.
var ErrNoProject = errors.New("no project for this directory and no current project set")

// WidgetUseCases groups all widget-related use cases.
type WidgetUseCases struct {
	widget   widgetdomain.WidgetRepository
	detector projectdomain.ProjectDetector
}

// NewWidgetUseCases constructs a WidgetUseCases instance.
// detector may be nil when CWD-based project detection is not needed.
func NewWidgetUseCases(
	widget widgetdomain.WidgetRepository,
	detector projectdomain.ProjectDetector,
) *WidgetUseCases {
	return &WidgetUseCases{widget: widget, detector: detector}
}

// GetSummary returns the widget summary for the project that applies to cwd.
// The project whose associated_path is the longest path prefix of cwd wins;
// when nothing matches (or no detector is configured) fallbackProjectID, the
// configured current project, is used. ErrNoProject is returned when neither
// yields a project.
func (uc *WidgetUseCases) GetSummary(
	ctx context.Context,
	fallbackProjectID, actorID, cwd string,
) (widgetdomain.Summary, error) {
	projectID, err := uc.ResolveProject(cwd, fallbackProjectID)
	if err != nil {
		return widgetdomain.Summary{}, err
	}
	return uc.widget.GetSummary(ctx, projectID, actorID)
}

// ResolveProject picks the project ID for cwd, falling back to
// fallbackProjectID. See GetSummary for the precedence rules.
func (uc *WidgetUseCases) ResolveProject(cwd, fallbackProjectID string) (string, error) {
	if uc.detector != nil && cwd != "" {
		detected, err := uc.detector.DetectForPath(cwd)
		if err != nil {
			return "", fmt.Errorf("detect project for path %q: %w", cwd, err)
		}
		if detected != "" {
			return detected, nil
		}
	}
	if fallbackProjectID != "" {
		return fallbackProjectID, nil
	}
	return "", ErrNoProject
}
