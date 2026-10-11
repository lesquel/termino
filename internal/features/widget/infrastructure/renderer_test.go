package infrastructure

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
)

// updateGolden is set via -update flag to regenerate golden files.
var updateGolden = flag.Bool("update", false, "regenerate golden files")

// ansiRe matches ANSI escape sequences so we can strip them for deterministic
// golden-file comparison across environments.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// goldenCompare compares actual against a golden file, updating it on -update.
func goldenCompare(t *testing.T, name, actual string) {
	t.Helper()
	plain := stripANSI(actual)
	goldenPath := filepath.Join("testdata", name+".golden")

	if *updateGolden {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(plain), 0644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", goldenPath, err)
	}
	if plain != string(want) {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s",
			name, want, plain)
	}
}

func mockSummary() widgetdomain.Summary {
	due := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	return widgetdomain.Summary{
		ProjectName: "Termino",
		ProjectKey:  "TER",
		AssignedTasks: []widgetdomain.TaskSummary{
			{ID: "t1", Ref: "TER-1", Title: "Implement widget renderers", ColumnName: "In Progress"},
			{ID: "t2", Ref: "TER-2", Title: "Write tests for adapters", DueDate: &due, ColumnName: "Backlog"},
		},
		InProgressTasks: []widgetdomain.TaskSummary{
			{ID: "t1", Ref: "TER-1", Title: "Implement widget renderers", ColumnName: "In Progress"},
			{ID: "t3", Ref: "TER-3", Title: "Fix the polling bug in the board", ColumnName: "In Progress"},
		},
		DueSoonTasks: []widgetdomain.TaskSummary{
			{ID: "t2", Ref: "TER-2", Title: "Write tests for adapters", DueDate: &due, ColumnName: "Backlog"},
		},
	}
}

func TestCompactRenderer(t *testing.T) {
	r := NewCompactRenderer()
	out, err := r.Render(mockSummary(), 50)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenCompare(t, "compact_width50", out)
}

func TestNormalRenderer(t *testing.T) {
	r := NewNormalRenderer()
	out, err := r.Render(mockSummary(), 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenCompare(t, "normal_width80", out)
}

func TestWideRenderer(t *testing.T) {
	r := NewWideRenderer()
	out, err := r.Render(mockSummary(), 130)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenCompare(t, "wide_width130", out)
}

func TestCompactRendererEmptySummary(t *testing.T) {
	r := NewCompactRenderer()
	out, err := r.Render(widgetdomain.Summary{ProjectName: "Empty", ProjectKey: "EMP"}, 50)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if stripANSI(out) == "" {
		t.Error("expected non-empty compact output for empty summary")
	}
}

func TestNormalRendererEmptyTasks(t *testing.T) {
	r := NewNormalRenderer()
	summary := widgetdomain.Summary{ProjectName: "Empty", ProjectKey: "EMP"}
	out, err := r.Render(summary, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if stripANSI(out) == "" {
		t.Error("expected non-empty normal output for empty summary")
	}
}

func TestWideRendererEmptyTasks(t *testing.T) {
	r := NewWideRenderer()
	summary := widgetdomain.Summary{ProjectName: "Empty", ProjectKey: "EMP"}
	out, err := r.Render(summary, 130)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if stripANSI(out) == "" {
		t.Error("expected non-empty wide output for empty summary")
	}
}
