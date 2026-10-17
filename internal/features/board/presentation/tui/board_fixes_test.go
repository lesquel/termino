package tui_test

import (
	"strings"
	"testing"

	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// TestBoardSelectedCardHasTextMarkerWithoutColor proves the selection stays
// visible under NO_COLOR, where background color is dropped.
func TestBoardSelectedCardHasTextMarkerWithoutColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, uc := fixture(t, baseTasks(), nil)
	view := newLoadedModel(t, uc, boardtui.Services{}, 120).View()
	if !strings.Contains(view, "> #1 Fix login") {
		t.Errorf("selected card should carry a '>' marker:\n%s", view)
	}
	if strings.Contains(view, "> #2") {
		t.Errorf("only the selected card carries the marker:\n%s", view)
	}
}

// TestBoardMultiRuneKeysApplyEachRune proves keys delivered together in one
// KeyRunes message (fast typing, key repeat) are each applied.
func TestBoardMultiRuneKeysApplyEachRune(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := newLoadedModel(t, uc, boardtui.Services{}, 120)
	if got := press(t, m, "jj").(boardtui.BoardModel).SelectedTaskID(); got != "t2" {
		t.Errorf("jj should select t2, got %q", got)
	}
	if got := press(t, m, "ll").(boardtui.BoardModel).ColIndex(); got != 2 {
		t.Errorf("ll should select column 2, got %d", got)
	}
	// Text-input modes still receive the runes verbatim.
	if view := press(t, m, "/", "ll", "enter").View(); !strings.Contains(view, "Filter: ll") {
		t.Errorf("multi-rune input should be typed into the filter:\n%s", view)
	}
}

// TestBoardWIPHeaderFitsOnOneLine proves a long WIP badge never wraps the
// column header onto a second line.
func TestBoardWIPHeaderFitsOnOneLine(t *testing.T) {
	limit := 1
	tasks := []taskdomain.Task{
		{ID: "a", ProjectID: "proj-1", Number: 1, Title: "A", ColumnID: "col-2", Position: 1},
		{ID: "b", ProjectID: "proj-1", Number: 2, Title: "B", ColumnID: "col-2", Position: 2},
	}
	_, uc := fixture(t, tasks, &limit)
	view := newLoadedModel(t, uc, boardtui.Services{}, 80).View()
	found := false
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "2/1") {
			found = true
			if !strings.Contains(line, "In") || !strings.Contains(line, "]") {
				t.Errorf("WIP badge wrapped away from the column name: %q\n%s", line, view)
			}
		}
	}
	if !found {
		t.Fatalf("WIP badge missing:\n%s", view)
	}
}
