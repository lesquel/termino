// Package domain defines the task source strategy interface and its value
// objects. No imports outside the Go standard library are used.
package domain

// SourceFile identifies a file discovered by a TaskSource adapter.
type SourceFile struct {
	Path        string
	AdapterName string
}

// SourceTask is a task parsed from a source file.
type SourceTask struct {
	Ref         string
	Title       string
	Description string
	Done        bool
}

// TaskSource is the strategy interface implemented by each adapter
// (kirospec, markdown, etc.).
type TaskSource interface {
	// Name returns the unique identifier for this adapter (e.g. "kirospec").
	Name() string
	// Detect scans root and returns the source files it can handle.
	Detect(root string) ([]SourceFile, error)
	// Parse reads a source file and returns the tasks it contains.
	Parse(file SourceFile) ([]SourceTask, error)
	// WriteStatus updates the done/undone status of a task in the source file.
	WriteStatus(file SourceFile, ref string, done bool) error
}
