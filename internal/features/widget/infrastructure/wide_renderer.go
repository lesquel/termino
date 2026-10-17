package infrastructure

import (
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// WideRenderer renders the widget for wide terminals (width > 120): the
// normal layout with aligned title, due-date and column fields. The due
// field appears only when a listed task has a due date.
type WideRenderer struct{}

// NewWideRenderer returns a WideRenderer.
func NewWideRenderer() *WideRenderer { return &WideRenderer{} }

// Render formats summary with ref, title, due date and column per task.
func (r *WideRenderer) Render(summary widgetdomain.Summary, width int) (string, error) {
	return renderList(summary, width, true), nil
}
