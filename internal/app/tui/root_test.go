package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	apptui "github.com/lesquel/termino/internal/app/tui"
	"github.com/lesquel/termino/internal/platform/config"
)

func buildMinimalRootDeps() apptui.RootDeps {
	cfg := &config.Config{
		Theme:            "default",
		CurrentProjectID: "",
	}
	// Load the embedded default theme.
	loadedTheme, _ := apptui.Load("default")
	return apptui.RootDeps{
		Config: cfg,
		Theme:  loadedTheme,
		// ProjectUC/BoardUC/TaskUC are nil — this tests model init without crash.
	}
}

func TestRootModelInit_NoProject_ShowsSelector(t *testing.T) {
	deps := buildMinimalRootDeps()
	model := apptui.NewRootModel(deps)

	tm := teatest.NewTestModel(
		t,
		model,
		teatest.WithInitialTermSize(120, 40),
	)
	// Wait for any output.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return len(bts) > 0
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestRootModelQuit(t *testing.T) {
	deps := buildMinimalRootDeps()
	model := apptui.NewRootModel(deps)

	tm := teatest.NewTestModel(
		t,
		model,
		teatest.WithInitialTermSize(120, 40),
	)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}
