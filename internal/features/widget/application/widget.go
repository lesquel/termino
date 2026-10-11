// Package application contains the widget feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"fmt"

	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

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

// GetSummary returns the widget summary for a project.
// If projectID is empty and a detector is configured, it will attempt to
// resolve the project from the given cwd path.
func (uc *WidgetUseCases) GetSummary(
	ctx context.Context,
	projectID, actorID, cwd string,
) (widgetdomain.Summary, error) {
	if projectID == "" {
		if uc.detector == nil {
			return widgetdomain.Summary{}, fmt.Errorf("no project ID provided and no project detector configured")
		}
		resolved, err := uc.detector.DetectForPath(cwd)
		if err != nil {
			return widgetdomain.Summary{}, fmt.Errorf("detect project for path %q: %w", cwd, err)
		}
		projectID = resolved
	}
	return uc.widget.GetSummary(ctx, projectID, actorID)
}
