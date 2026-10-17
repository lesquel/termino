package infrastructure

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileStore reads and writes shared files on the local filesystem. Writes
// are atomic: the content goes to a hidden ".<name>.tmp" sibling first and
// is renamed over the target, so readers (and git) never see a partial file.
type FileStore struct{}

// Write atomically replaces the file at path with data, creating parent
// directories as needed.
func (FileStore) Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("share: mkdir %s: %w", dir, err)
	}
	tmp := filepath.Join(dir, "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("share: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("share: rename %s: %w", path, err)
	}
	return nil
}

// Read returns the content of the file at path. A missing file yields an
// error wrapping os.ErrNotExist.
func (FileStore) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Remove deletes the file at path; a file that is already gone is not an error.
func (FileStore) Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("share: remove %s: %w", path, err)
	}
	return nil
}

// List returns the sorted names of the regular files in dir, skipping hidden
// files and leftovers ending in ".tmp". A missing dir yields no names.
func (FileStore) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("share: list %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ErrNotGitRepository is returned by FindGitRoot outside a git work tree.
var ErrNotGitRepository = errors.New("not inside a git repository")

// FindGitRoot walks up from start to the nearest directory holding a ".git"
// entry (a directory, or a file in worktrees and submodules). It needs no git
// binary.
func FindGitRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("share: resolve %s: %w", start, err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: %s", ErrNotGitRepository, start)
		}
		dir = parent
	}
}
