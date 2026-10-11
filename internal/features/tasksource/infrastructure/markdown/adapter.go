// Package markdown provides a TaskSource adapter for generic markdown files
// that contain checkbox task items (- [ ] / - [x] lines).
package markdown

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tasksourcedomain "github.com/lesquel/termino/internal/features/tasksource/domain"
)

// Adapter implements tasksourcedomain.TaskSource for generic markdown files.
type Adapter struct{}

// New returns an Adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the unique identifier for this adapter.
func (a *Adapter) Name() string { return "markdown" }

// Detect scans root for .md files that contain at least one checkbox line.
func (a *Adapter) Detect(root string) ([]tasksourcedomain.SourceFile, error) {
	var files []tasksourcedomain.SourceFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}
		if d.IsDir() {
			// Skip hidden directories and common non-project dirs.
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		if hasCheckbox(path) {
			files = append(files, tasksourcedomain.SourceFile{
				Path:        path,
				AdapterName: a.Name(),
			})
		}
		return nil
	})
	return files, err
}

// Parse reads a markdown file and returns SourceTasks for every checkbox line.
func (a *Adapter) Parse(file tasksourcedomain.SourceFile) ([]tasksourcedomain.SourceTask, error) {
	relPath := relativeOrAbs(file.Path)

	f, err := os.Open(file.Path)
	if err != nil {
		return nil, fmt.Errorf("markdown.Parse open: %w", err)
	}
	defer f.Close()

	var tasks []tasksourcedomain.SourceTask
	scanner := bufio.NewScanner(f)
	lineIdx := 0
	checkboxIdx := 0
	for scanner.Scan() {
		line := scanner.Text()
		done, title, ok := parseCheckbox(line)
		if ok {
			b64 := base64.StdEncoding.EncodeToString([]byte(relPath))
			tasks = append(tasks, tasksourcedomain.SourceTask{
				Ref:   fmt.Sprintf("md:%s:%d", b64, checkboxIdx),
				Title: title,
				Done:  done,
			})
			checkboxIdx++
		}
		lineIdx++
	}
	return tasks, scanner.Err()
}

// WriteStatus atomically toggles the checkbox identified by ref.
// ref format: md:<base64(relativePath)>:<checkboxIndex>
func (a *Adapter) WriteStatus(file tasksourcedomain.SourceFile, ref string, done bool) error {
	_, idx, err := parseRef(ref)
	if err != nil {
		return fmt.Errorf("markdown.WriteStatus: invalid ref %q: %w", ref, err)
	}

	data, err := os.ReadFile(file.Path)
	if err != nil {
		return fmt.Errorf("markdown.WriteStatus read: %w", err)
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

// hasCheckbox returns true if the file contains at least one checkbox line.
func hasCheckbox(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			return true
		}
	}
	return false
}

// parseCheckbox returns (done, title, ok).
func parseCheckbox(line string) (done bool, title string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- [ ] ") {
		return false, strings.TrimPrefix(trimmed, "- [ ] "), true
	}
	if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
		t := strings.TrimPrefix(trimmed, "- [x] ")
		t = strings.TrimPrefix(t, "- [X] ")
		return true, t, true
	}
	return false, "", false
}

// toggleCheckbox replaces the checkbox state in a line.
func toggleCheckbox(line string, done bool) string {
	if done {
		line = strings.Replace(line, "- [ ] ", "- [x] ", 1)
	} else {
		line = strings.Replace(line, "- [x] ", "- [ ] ", 1)
		line = strings.Replace(line, "- [X] ", "- [ ] ", 1)
	}
	return line
}

// parseRef parses md:<b64path>:<idx>.
func parseRef(ref string) (relPath string, idx int, err error) {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 || parts[0] != "md" {
		return "", 0, fmt.Errorf("expected md:<b64>:<index>, got %q", ref)
	}
	b, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("decode base64 path: %w", err)
	}
	i, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, fmt.Errorf("parse index: %w", err)
	}
	return string(b), i, nil
}

// relativeOrAbs returns the path as-is (relative path is preferred for refs
// but we store absolute for file ops). For ref encoding, we use the basename
// path.
func relativeOrAbs(path string) string {
	return filepath.ToSlash(path)
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
