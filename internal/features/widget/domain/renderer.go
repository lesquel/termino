// Package domain defines the widget summary value objects, repository
// interface, and the Renderer strategy interface.
// No imports outside the Go standard library are used.
package domain

// Renderer formats a Summary into a displayable string.
// Implementations live in infrastructure/ to allow external libraries.
type Renderer interface {
	// Render formats summary for the given terminal width.
	Render(summary Summary, width int) (string, error)
}
