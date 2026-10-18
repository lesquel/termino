package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/tuikit"
)

// refresh delivers one poll tick and runs the data-version check it starts
// (not the next tick, which would sleep).
func refresh(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	m, cmd := m.Update(tuikit.RefreshMsg{})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("refresh returned %T, want a batch", cmd())
	}
	m, next := m.Update(batch[0]())
	return run(t, m, next)
}

// TestBoardReloadsAfterSharedImport: the share watcher imports on the
// board's own DB connection, which data_version does not report, so the
// board also reloads when the share version moves.
func TestBoardReloadsAfterSharedImport(t *testing.T) {
	repo, uc := fixture(t, baseTasks(), nil)
	var version int64
	svc := boardtui.Services{ShareVersion: func() int64 { return version }}
	m := newProjectModel(t, uc, svc, 120, nil)

	repo.tasks = append(repo.tasks, taskdomain.Task{ID: "t9", ProjectID: "proj-1", Number: 9,
		Title: "From a teammate", ColumnID: "col-1", Priority: taskdomain.PriorityLow, Position: 3})
	m = refresh(t, m)
	if strings.Contains(ansi.Strip(m.View()), "From a teammate") {
		t.Fatal("board reloaded without a change")
	}
	version++
	m = refresh(t, m)
	if !strings.Contains(ansi.Strip(m.View()), "WEB-9 From a teammate") {
		t.Errorf("board did not reload after a shared import:\n%s", ansi.Strip(m.View()))
	}
}
