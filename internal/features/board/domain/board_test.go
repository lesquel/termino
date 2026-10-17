package domain_test

import (
	"testing"

	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
)

func TestDoneColumn(t *testing.T) {
	tests := []struct {
		name   string
		cols   []boarddomain.Column
		wantID string
		wantOK bool
	}{
		{name: "empty", cols: nil, wantOK: false},
		{
			name: "highest position wins regardless of order",
			cols: []boarddomain.Column{
				{ID: "done", Position: 2},
				{ID: "todo", Position: 0},
				{ID: "doing", Position: 1},
			},
			wantID: "done", wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := boarddomain.DoneColumn(tt.cols)
			if ok != tt.wantOK || got.ID != tt.wantID {
				t.Errorf("DoneColumn() = (%q, %v), want (%q, %v)", got.ID, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestWIPExceeded(t *testing.T) {
	limit := 2
	col := boarddomain.Column{ID: "c", WIPLimit: &limit}
	board := boarddomain.Board{Tasks: map[string][]string{"c": {"a", "b", "c"}}}
	if !boarddomain.WIPExceeded(board, col) {
		t.Error("3 tasks over a limit of 2 should exceed WIP")
	}
	board.Tasks["c"] = []string{"a", "b"}
	if boarddomain.WIPExceeded(board, col) {
		t.Error("2 tasks at a limit of 2 should not exceed WIP")
	}
	if boarddomain.WIPExceeded(board, boarddomain.Column{ID: "c"}) {
		t.Error("a column without a limit never exceeds WIP")
	}
}
