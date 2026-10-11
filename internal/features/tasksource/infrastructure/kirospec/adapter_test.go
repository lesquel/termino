package kirospec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
)

func TestKiroSpecAdapter_Name(t *testing.T) {
	a := New()
	if got := a.Name(); got != "kiro-spec" {
		t.Errorf("Name() = %q, want %q", got, "kiro-spec")
	}
}

func TestKiroSpecAdapter_Detect(t *testing.T) {
	root := t.TempDir()

	// Create .kiro/specs/myfeature/tasks.md and a non-matching file.
	specDir := filepath.Join(root, ".kiro", "specs", "myfeature")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.WriteFile(tasksFile, []byte("- [ ] task one\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Add a second feature.
	specDir2 := filepath.Join(root, ".kiro", "specs", "otherfeature")
	if err := os.MkdirAll(specDir2, 0755); err != nil {
		t.Fatalf("mkdir2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir2, "tasks.md"), []byte("- [ ] task two\n"), 0644); err != nil {
		t.Fatalf("write2: %v", err)
	}

	a := New()
	files, err := a.Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 detected files, got %d", len(files))
	}
	for _, f := range files {
		if f.AdapterName != "kiro-spec" {
			t.Errorf("adapter name = %q, want %q", f.AdapterName, "kiro-spec")
		}
		if !strings.HasSuffix(f.Path, "tasks.md") {
			t.Errorf("path = %q, want ending in tasks.md", f.Path)
		}
	}
}

func TestKiroSpecAdapter_Parse(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, ".kiro", "specs", "myfeature")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	content := "# My Feature\n\nSome intro text.\n\n- [ ] First task\n- [x] Second task done\n- [ ] Third task\n\nMore text.\n"
	if err := os.WriteFile(tasksFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	tasks, err := a.Parse(tasksourcedomain.SourceFile{Path: tasksFile, AdapterName: "kiro-spec"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// First task: undone.
	if tasks[0].Done {
		t.Errorf("task[0].Done = true, want false")
	}
	if tasks[0].Title != "First task" {
		t.Errorf("task[0].Title = %q, want %q", tasks[0].Title, "First task")
	}
	if tasks[0].Ref != "kiro:myfeature:0" {
		t.Errorf("task[0].Ref = %q, want %q", tasks[0].Ref, "kiro:myfeature:0")
	}

	// Second task: done.
	if !tasks[1].Done {
		t.Errorf("task[1].Done = false, want true")
	}
	if tasks[1].Title != "Second task done" {
		t.Errorf("task[1].Title = %q, want %q", tasks[1].Title, "Second task done")
	}

	// Third task: undone.
	if tasks[2].Done {
		t.Errorf("task[2].Done = true, want false")
	}
}

func TestKiroSpecAdapter_WriteStatus_ToggleUndone(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, ".kiro", "specs", "myfeature")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	content := "- [ ] First task\n- [x] Second task\n"
	if err := os.WriteFile(tasksFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	sf := tasksourcedomain.SourceFile{Path: tasksFile, AdapterName: "kiro-spec"}

	// Toggle first task to done.
	if err := a.WriteStatus(sf, "kiro:myfeature:0", true); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	// Re-read and verify only the first checkbox changed.
	data, err := os.ReadFile(tasksFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "[x]") {
		t.Errorf("expected first line to have [x], got %q", lines[0])
	}
	if !strings.HasPrefix(lines[0], "- [x] First task") {
		t.Errorf("expected first line '- [x] First task', got %q", lines[0])
	}
	// Second line should be unchanged.
	if lines[1] != "- [x] Second task" {
		t.Errorf("expected second line unchanged, got %q", lines[1])
	}
}

func TestKiroSpecAdapter_WriteStatus_ToggleDone(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, ".kiro", "specs", "feat")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	content := "- [ ] First task\n- [x] Second task\n"
	if err := os.WriteFile(tasksFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	sf := tasksourcedomain.SourceFile{Path: tasksFile, AdapterName: "kiro-spec"}

	// Toggle second task to undone.
	if err := a.WriteStatus(sf, "kiro:feat:1", false); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	data, err := os.ReadFile(tasksFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// First line unchanged.
	if lines[0] != "- [ ] First task" {
		t.Errorf("expected first line unchanged, got %q", lines[0])
	}
	// Second line should now be [ ].
	if lines[1] != "- [ ] Second task" {
		t.Errorf("expected second line '- [ ] Second task', got %q", lines[1])
	}
}

func TestKiroSpecAdapter_WriteStatus_AtomicRename(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, ".kiro", "specs", "f")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.WriteFile(tasksFile, []byte("- [ ] task\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	sf := tasksourcedomain.SourceFile{Path: tasksFile, AdapterName: "kiro-spec"}
	if err := a.WriteStatus(sf, "kiro:f:0", true); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	// Ensure no temp files left behind.
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".termino-tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
