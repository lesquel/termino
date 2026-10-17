package infrastructure

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
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
		Columns: []widgetdomain.ColumnCount{
			{Name: "Backlog", Count: 1}, {Name: "In Progress", Count: 2}, {Name: "Done", Count: 4},
		},
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

// backlogOnly is the reported case: open work sits only in Backlog.
func backlogOnly() widgetdomain.Summary {
	return widgetdomain.Summary{
		ProjectName: "termino",
		ProjectKey:  "PROJ",
		Columns: []widgetdomain.ColumnCount{
			{Name: "Backlog", Count: 2}, {Name: "In Progress", Count: 0}, {Name: "Done", Count: 0},
		},
		NextTasks: []widgetdomain.TaskSummary{
			{ID: "p1", Ref: "PROJ-1", Title: "Design the landing page", ColumnName: "Backlog"},
			{ID: "p2", Ref: "PROJ-2", Title: "Write the onboarding guide", ColumnName: "Backlog"},
		},
	}
}

func renderers() map[string]widgetdomain.Renderer {
	return map[string]widgetdomain.Renderer{
		"compact": NewCompactRenderer(),
		"normal":  NewNormalRenderer(),
		"wide":    NewWideRenderer(),
	}
}

var widths = map[string]int{"compact": 50, "normal": 80, "wide": 130}

func TestRenderersShowBacklogWhenNothingIsActive(t *testing.T) {
	for name, r := range renderers() {
		out, err := r.Render(backlogOnly(), widths[name])
		if err != nil {
			t.Fatalf("%s: Render: %v", name, err)
		}
		plain := stripANSI(out)
		for _, want := range []string{"termino (PROJ)", "Backlog 2 · In Progress 0 · Done 0", "PROJ-1"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%s: missing %q in:\n%s", name, want, plain)
			}
		}
		if strings.Contains(plain, "no active tasks") {
			t.Errorf("%s: claims no active tasks while Backlog has work:\n%s", name, plain)
		}
		if !strings.Contains(plain, "termino board") {
			t.Errorf("%s: missing board hint:\n%s", name, plain)
		}
	}
}

func TestNormalRendererBacklogGolden(t *testing.T) {
	out, _ := NewNormalRenderer().Render(backlogOnly(), 80)
	goldenCompare(t, "normal_backlog_width80", out)
}

func TestWideRendererDropsDueWithoutDueDates(t *testing.T) {
	out, _ := NewWideRenderer().Render(backlogOnly(), 130)
	plain := stripANSI(out)
	if strings.Contains(plain, "—") || strings.Contains(plain, "DUE") {
		t.Errorf("due field shown although no task has a due date:\n%s", plain)
	}
	goldenCompare(t, "wide_backlog_width130", out)
}

func TestRenderersEmptyProjectHint(t *testing.T) {
	empty := widgetdomain.Summary{
		ProjectName: "Empty", ProjectKey: "EMP",
		Columns: []widgetdomain.ColumnCount{{Name: "Backlog"}, {Name: "Done"}},
	}
	for name, r := range renderers() {
		out, err := r.Render(empty, widths[name])
		if err != nil {
			t.Fatalf("%s: Render: %v", name, err)
		}
		if !strings.Contains(stripANSI(out), emptyHint) {
			t.Errorf("%s: missing empty hint in:\n%s", name, stripANSI(out))
		}
	}
}

func TestRenderersAllDoneHint(t *testing.T) {
	done := widgetdomain.Summary{
		ProjectName: "Shipped", ProjectKey: "SHP",
		Columns: []widgetdomain.ColumnCount{{Name: "Backlog"}, {Name: "Done", Count: 3}},
	}
	for name, r := range renderers() {
		out, _ := r.Render(done, widths[name])
		if !strings.Contains(stripANSI(out), nothingOpenHint) {
			t.Errorf("%s: missing nothing-open hint in:\n%s", name, stripANSI(out))
		}
	}
}

func TestRenderersStayWithinWidth(t *testing.T) {
	long := mockSummary()
	long.InProgressTasks[1].Title = strings.Repeat("very long title ", 20)
	for name, r := range renderers() {
		w := widths[name]
		out, _ := r.Render(long, w)
		for _, line := range strings.Split(stripANSI(out), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("%s: line is %d cells, width %d: %q", name, got, w, line)
			}
		}
	}
}
