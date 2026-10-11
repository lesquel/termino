// Package kirospec provides a TaskSource adapter for Kiro spec files at
// .kiro/specs/<feature>/tasks.md.
package kirospec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
)

// Adapter implements tasksourcedomain.TaskSource for Kiro spec task files.
type Adapter struct{}

// New returns an Adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the unique identifier for this adapter.
func (a *Adapter) Name() string { return "kiro-spec" }

// Detect scans root for .kiro/specs/*/tasks.md files.
func (a *Adapter) Detect(root string) ([]tasksourcedomain.SourceFile, error) {
	pattern := filepath.Join(root, ".kiro", "specs", "*", "tasks.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("kirospec.Detect glob: %w", err)
	}
	files := make([]tasksourcedomain.SourceFile, 0, len(matches))
	for _, m := range matches {
		files = append(files, tasksourcedomain.SourceFile{
			Path:        m,
			AdapterName: a.Name(),
		})
	}
	return files, nil
}

// Parse reads a tasks.md file and returns SourceTasks for every checkbox line.
func (a *Adapter) Parse(file tasksourcedomain.SourceFile) ([]tasksourcedomain.SourceTask, error) {
	feature := extractFeature(file.Path)

	f, err := os.Open(file.Path)
	if err != nil {
		return nil, fmt.Errorf("kirospec.Parse open: %w", err)
	}
	defer f.Close()

	var tasks []tasksourcedomain.SourceTask
	scanner := bufio.NewScanner(f)
	idx := 0
	for scanner.Scan() {
		line := scanner.Text()
		done, title, ok := parseCheckbox(line)
		if !ok {
			continue
		}
		tasks = append(tasks, tasksourcedomain.SourceTask{
			Ref:   fmt.Sprintf("kiro:%s:%d", feature, idx),
			Title: title,
			Done:  done,
		})
		idx++
	}
	return tasks, scanner.Err()
}

// WriteStatus atomically toggles the checkbox for the task identified by ref.
// ref format: kiro:<feature>:<index>
func (a *Adapter) WriteStatus(file tasksourcedomain.SourceFile, ref string, done bool) error {
	idx, err := refIndex(ref)
	if err != nil {
		return fmt.Errorf("kirospec.WriteStatus: invalid ref %q: %w", ref, err)
	}

	data, err := os.ReadFile(file.Path)
	if err != nil {
		return fmt.Errorf("kirospec.WriteStatus read: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	checkboxIdx := 0
	for i, line := range lines {
		_, _, ok := parseCheckbox(line)
		if !ok {
			continue
		}
		if checkboxIdx == idx {
			lines[i] = toggleCheckbox(line, done)
			break
		}
		checkboxIdx++
	}

	newContent := strings.Join(lines, "\n")
	return atomicWrite(file.Path, []byte(newContent))
}

// extractFeature derives the feature name from path .kiro/specs/<feature>/tasks.md.
func extractFeature(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "specs" && i+2 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}

// parseCheckbox returns (done, title, ok) for lines like "- [ ] text" or "- [x] text".
func parseCheckbox(line string) (done bool, title string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- [ ] ") {
		return false, strings.TrimPrefix(trimmed, "- [ ] "), true
	}
	if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
		return true, strings.TrimPrefix(trimmed, "- [x] "), true
	}
	return false, "", false
}

// toggleCheckbox replaces the checkbox state in a line.
func toggleCheckbox(line string, done bool) string {
	if done {
		line = strings.Replace(line, "- [ ] ", "- [x] ", 1)
		line = strings.Replace(line, "- [X] ", "- [x] ", 1)
	} else {
		line = strings.Replace(line, "- [x] ", "- [ ] ", 1)
		line = strings.Replace(line, "- [X] ", "- [ ] ", 1)
	}
	return line
}

// refIndex parses the integer index from a kiro ref.
func refIndex(ref string) (int, error) {
	parts := strings.Split(ref, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("expected kiro:<feature>:<index>, got %q", ref)
	}
	return strconv.Atoi(parts[2])
}

// atomicWrite writes data to path via a temp file + os.Rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".termino-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
