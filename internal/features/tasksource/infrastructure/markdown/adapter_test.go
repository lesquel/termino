package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
)

func TestMarkdownAdapter_Name(t *testing.T) {
	a := New()
	if got := a.Name(); got != "markdown" {
		t.Errorf("Name() = %q, want %q", got, "markdown")
	}
}

func TestMarkdownAdapter_Detect(t *testing.T) {
	root := t.TempDir()

	// File with checkboxes — should be detected.
	mdFile := filepath.Join(root, "TODO.md")
	if err := os.WriteFile(mdFile, []byte("# TODO\n- [ ] task one\n- [x] done\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// File without checkboxes — should be ignored.
	plainFile := filepath.Join(root, "README.md")
	if err := os.WriteFile(plainFile, []byte("# README\nJust text\n"), 0644); err != nil {
		t.Fatalf("write2: %v", err)
	}

	a := New()
	files, err := a.Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 detected file, got %d", len(files))
	}
	if files[0].Path != mdFile {
		t.Errorf("expected %q, got %q", mdFile, files[0].Path)
	}
}

func TestMarkdownAdapter_DetectSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	hiddenDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	hiddenFile := filepath.Join(hiddenDir, "notes.md")
	if err := os.WriteFile(hiddenFile, []byte("- [ ] hidden task\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	files, err := a.Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files from hidden dir, got %d", len(files))
	}
}

func TestMarkdownAdapter_Parse(t *testing.T) {
	root := t.TempDir()
	mdFile := filepath.Join(root, "tasks.md")
	content := "# Tasks\n\n- [ ] First\n- [x] Second done\n- [ ] Third\n"
	if err := os.WriteFile(mdFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	tasks, err := a.Parse(tasksourcedomain.SourceFile{Path: mdFile, AdapterName: "markdown"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Done {
		t.Error("task[0] should not be done")
	}
	if tasks[0].Title != "First" {
		t.Errorf("task[0].Title = %q, want %q", tasks[0].Title, "First")
	}
	if !tasks[1].Done {
		t.Error("task[1] should be done")
	}
	if !strings.HasPrefix(tasks[1].Ref, "md:") {
		t.Errorf("task[1].Ref = %q, want prefix md:", tasks[1].Ref)
	}
}

func TestMarkdownAdapter_WriteStatus_ToggleUndone(t *testing.T) {
	root := t.TempDir()
	mdFile := filepath.Join(root, "tasks.md")
	content := "- [ ] First task\n- [x] Second task\n"
	if err := os.WriteFile(mdFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	sf := tasksourcedomain.SourceFile{Path: mdFile, AdapterName: "markdown"}

	// Parse to get the ref for the first task.
	tasks, err := a.Parse(sf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	firstRef := tasks[0].Ref

	// Toggle first to done.
	if err := a.WriteStatus(sf, firstRef, true); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	data, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !strings.HasPrefix(lines[0], "- [x] First task") {
		t.Errorf("expected first line '- [x] First task', got %q", lines[0])
	}
	// Second line unchanged.
	if lines[1] != "- [x] Second task" {
		t.Errorf("expected second line unchanged, got %q", lines[1])
	}
}

func TestMarkdownAdapter_WriteStatus_ToggleDone(t *testing.T) {
	root := t.TempDir()
	mdFile := filepath.Join(root, "tasks.md")
	content := "- [ ] First task\n- [x] Second task\n"
	if err := os.WriteFile(mdFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	sf := tasksourcedomain.SourceFile{Path: mdFile, AdapterName: "markdown"}

	tasks, err := a.Parse(sf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	secondRef := tasks[1].Ref

	// Toggle second to undone.
	if err := a.WriteStatus(sf, secondRef, false); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	data, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// First line unchanged.
	if lines[0] != "- [ ] First task" {
		t.Errorf("expected first line unchanged, got %q", lines[0])
	}
	if lines[1] != "- [ ] Second task" {
		t.Errorf("expected second line '- [ ] Second task', got %q", lines[1])
	}
}

func TestMarkdownAdapter_WriteStatus_OnlyTargetChanges(t *testing.T) {
	root := t.TempDir()
	mdFile := filepath.Join(root, "tasks.md")
	content := "- [ ] A\n- [ ] B\n- [ ] C\n"
	if err := os.WriteFile(mdFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := New()
	sf := tasksourcedomain.SourceFile{Path: mdFile, AdapterName: "markdown"}

	tasks, err := a.Parse(sf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Toggle the middle task (index 1) to done.
	if err := a.WriteStatus(sf, tasks[1].Ref, true); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	data, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if lines[0] != "- [ ] A" {
		t.Errorf("line 0 changed: %q", lines[0])
	}
	if lines[1] != "- [x] B" {
		t.Errorf("line 1 should be [x]: %q", lines[1])
	}
	if lines[2] != "- [ ] C" {
		t.Errorf("line 2 changed: %q", lines[2])
	}
}
