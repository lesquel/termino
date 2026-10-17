// Package domain defines the on-disk snapshot of a shared project: the
// project.toml and per-task TOML files kept under <repo>/.termino/. No imports
// outside the Go standard library are used.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"time"
)

// FormatVersion is the version written to every shared file. Files with a
// higher version come from a newer termino and are skipped on import.
const FormatVersion = 1

// Layout of the shared directory, relative to the share root (git root).
const (
	// Dir is the directory under the share root that holds the shared files.
	Dir = ".termino"
	// ProjectFileName is the project file inside Dir.
	ProjectFileName = "project.toml"
	// TasksDir is the directory inside Dir holding one file per task.
	TasksDir = "tasks"
)

// TaskFilePath returns the path of a task's file relative to Dir. Files are
// named by task UUID so concurrent creates never collide in git.
func TaskFilePath(taskID string) string {
	return path.Join(TasksDir, taskID+".toml")
}

// Errors returned by the decoder.
var (
	// ErrConflictMarkers means the file still holds unresolved git conflict markers.
	ErrConflictMarkers = errors.New("file has unresolved git conflict markers")
	// ErrInvalidFile means the file is not valid TOML or misses required keys.
	ErrInvalidFile = errors.New("invalid shared file")
	// ErrUnsupportedVersion means the file was written by a newer format version.
	ErrUnsupportedVersion = errors.New("unsupported shared file version")
)

// ProjectFile is the content of project.toml.
type ProjectFile struct {
	Version     int
	Key         string
	Name        string
	Methodology string
	Columns     []ColumnEntry
	Members     []MemberEntry
	Labels      []LabelEntry
	Sprints     []SprintEntry
}

// ColumnEntry is one board column. WIPLimit 0 means no limit.
type ColumnEntry struct {
	Name     string
	Position int
	WIPLimit int
}

// MemberEntry is one member, referenced by name from task files.
type MemberEntry struct {
	Name string
	Kind string
}

// LabelEntry is one project label. Color is optional.
type LabelEntry struct {
	Name  string
	Color string
}

// SprintEntry is one sprint. Start and End are calendar dates (UTC midnight);
// zero values are omitted.
type SprintEntry struct {
	Name  string
	Start time.Time
	End   time.Time
	State string
}

// TaskFile is the content of tasks/<id>.toml. The task reference (KEY-N) is
// not stored: it is always derived from the project key and Number. Comments,
// activity and task-source fields stay local and are never part of it.
type TaskFile struct {
	Version     int
	ID          string
	Number      int
	Title       string
	Column      string // column name
	Position    float64
	Priority    string
	Points      *int
	Assignee    string   // member name; "" when unassigned
	Labels      []string // label names
	Sprint      string   // sprint name; "" when none
	Due         *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	UpdatedBy   string
	Description string
}

// LosesNumber reports whether a must give up its number to b when both claim
// the same one: the task created later loses, and a tie is broken by the
// greater UUID. Every machine applies the same rule to the same exported
// timestamps, so all of them converge on the same numbering.
func LosesNumber(a, b TaskFile) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

// ContentHash returns the hex SHA-256 of data, used to detect changed files.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
