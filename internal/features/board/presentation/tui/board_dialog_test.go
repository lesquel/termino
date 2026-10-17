package tui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	boardtui "github.com/lesquel/termino/internal/features/board/presentation/tui"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	"github.com/lesquel/termino/internal/tuikit"
	"github.com/lesquel/termino/internal/tuikit/tuikittest"
)

// Every key the board switches on is listed in the help dialog.
func TestBoardKeysAreDocumented(t *testing.T) {
	tuikittest.AssertDocumented(t,
		tuikittest.CaseKeys(t, "board.go", "handleKey"),
		tuikit.HelpNavigate, tuikit.HelpTasks, tuikit.HelpViews)
	tuikittest.AssertDocumented(t,
		tuikittest.CaseKeys(t, "board.go", "handlePickerKey"),
		tuikit.HelpPickers)
}

// The pickers and the delete prompt are dialogs drawn over the board: the
// board header and other columns stay visible around them.
func TestBoardPickersAreDialogsOverTheBoard(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	members := fakeMembers{members: []memberdomain.Member{{ID: "m-ana", Name: "ana", Kind: memberdomain.KindHuman}}}
	for _, k := range []string{"m", "a", "d"} {
		t.Run(k, func(t *testing.T) {
			m := newLoadedModel(t, uc, boardtui.Services{Tasks: &fakeTaskActions{}, Members: members}, 120)
			view := ansi.Strip(press(t, m, k).View())
			if !strings.Contains(view, "╭") || !strings.Contains(view, "esc cancel") && k != "d" {
				t.Errorf("%s: expected a dialog with a cancel badge:\n%s", k, view)
			}
			if !strings.Contains(view, "In Progress") || !strings.Contains(strings.Split(view, "\n")[0], "termino") {
				t.Errorf("%s: the board should stay visible behind the dialog:\n%s", k, view)
			}
			for _, l := range strings.Split(view, "\n") {
				if ansi.StringWidth(l) > 120 {
					t.Fatalf("%s: row is %d cells wide", k, ansi.StringWidth(l))
				}
			}
		})
	}
}

// The selected picker option keeps its text "> " marker.
func TestBoardPickerKeepsTextMarker(t *testing.T) {
	_, uc := fixture(t, baseTasks(), nil)
	m := press(t, newLoadedModel(t, uc, boardtui.Services{Tasks: &fakeTaskActions{}}, 120), "m", "j")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "> 2. In Progress") {
		t.Errorf("expected the marker on the second option:\n%s", view)
	}
}
