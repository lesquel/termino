package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	apptui "github.com/lesquel/termino/internal/app/tui"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/tuikit"
)

// fakeWidget records install/uninstall calls instead of touching rc files.
type fakeWidget struct {
	shell     setupapp.ShellInfo
	rc        string
	installed bool
	calls     []string
}

func (w *fakeWidget) Status() (setupapp.WidgetStatus, error) {
	st := setupapp.WidgetStatus{Shell: w.shell, Installed: w.installed}
	if w.shell.Supported {
		st.RCPath = w.rc
	}
	if w.installed {
		st.InstalledPath = w.rc
	}
	return st, nil
}

func (w *fakeWidget) Install() (string, error) {
	w.calls = append(w.calls, "install")
	if !w.shell.Supported {
		return "", errors.New(w.shell.Note)
	}
	w.installed = true
	return w.rc, nil
}

func (w *fakeWidget) Uninstall() (string, error) {
	w.calls = append(w.calls, "uninstall")
	w.installed = false
	return w.rc, nil
}

type settingsFixture struct {
	m      tea.Model
	deps   apptui.RootDeps
	widget *fakeWidget
	saved  config.Config
	saves  int
}

func newSettingsRoot(t *testing.T, shell setupapp.ShellInfo) *settingsFixture {
	t.Helper()
	cols, tasks := screenFixture()
	theme, err := apptui.Load("default")
	if err != nil {
		t.Fatal(err)
	}
	f := &settingsFixture{widget: &fakeWidget{shell: shell, rc: "/home/u/.bashrc"}}
	f.saved = config.Config{Theme: "default", Motion: true}
	f.deps = apptui.RootDeps{
		Config:  &config.Config{CurrentProjectID: "p1", Theme: "default", Motion: true},
		BoardUC: boardapp.NewBoardUseCases(rootBoardRepo{cols: cols}, rootTaskRepo{tasks: tasks}),
		Theme:   theme,
		Settings: apptui.SettingsDeps{
			Themes:    []string{"default", "nord", "spooky"},
			LoadTheme: func(name string) (*tuikit.Theme, error) { return tuikit.Load(name, "") },
			SaveConfig: func(mutate func(*config.Config)) error {
				mutate(&f.saved)
				f.saves++
				return nil
			},
			Widget: f.widget,
			Home:   "/home/u",
		},
	}
	var m tea.Model = apptui.NewRootModel(f.deps)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	f.m = loadBoardInto(t, m, f.deps)
	return f
}

// press sends keys one by one and feeds back the settings messages the
// returned commands produce (motion changes, closing).
func (f *settingsFixture) press(t *testing.T, keys ...string) tea.Cmd {
	t.Helper()
	var last tea.Cmd
	for _, k := range keys {
		msg := key(k)
		switch k {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		}
		var cmd tea.Cmd
		f.m, cmd = f.m.Update(msg)
		last = cmd
		if cmd == nil {
			continue
		}
		switch out := cmd().(type) {
		case apptui.MotionChangedMsg, apptui.SettingsClosedMsg:
			f.m, _ = f.m.Update(out)
		}
	}
	return last
}

func (f *settingsFixture) view() string { return ansi.Strip(f.m.View()) }

func TestSettingsOpensWithSAndCapturesGlobalKeys(t *testing.T) {
	f := newSettingsRoot(t, setupapp.ShellInfo{Name: "bash", Supported: true})
	f.press(t, "s")
	v := f.view()
	for _, want := range []string{"Settings", "Theme", "nord", "Shell widget", "bash", "~/.bashrc", "Motion", "esc back"} {
		if !strings.Contains(v, want) {
			t.Errorf("settings view lacks %q:\n%s", want, v)
		}
	}
	for _, k := range []string{"q", "p", "?", "n"} {
		if cmd := f.press(t, k); isQuit(cmd) {
			t.Fatalf("%q quit the TUI from settings", k)
		}
		if !strings.Contains(f.view(), "Shell widget") {
			t.Fatalf("%q left the settings screen:\n%s", k, f.view())
		}
	}
	f.press(t, "esc")
	if v := f.view(); strings.Contains(v, "Shell widget") || !strings.Contains(v, "Backlog") {
		t.Fatalf("esc did not return to the board:\n%s", v)
	}
}

func TestSettingsThemeAppliesLiveAndSaves(t *testing.T) {
	f := newSettingsRoot(t, setupapp.ShellInfo{Name: "bash", Supported: true})
	f.press(t, "s", "j", "enter")
	nord, _ := tuikit.Load("nord", "")
	if f.deps.Theme.Accent != nord.Accent {
		t.Errorf("running theme accent = %v, want nord %v", f.deps.Theme.Accent, nord.Accent)
	}
	if f.saved.Theme != "nord" || f.saves != 1 {
		t.Errorf("saved config = %+v after %d saves", f.saved, f.saves)
	}
	if !strings.Contains(f.view(), "✓ Theme set to nord") {
		t.Errorf("no confirmation:\n%s", f.view())
	}
}

func TestSettingsWidgetToggleInstallsAndRemoves(t *testing.T) {
	f := newSettingsRoot(t, setupapp.ShellInfo{Name: "bash", Supported: true})
	f.press(t, "s", "down", "down", "down", "enter")
	if !strings.Contains(f.view(), "✓ Widget installed in ~/.bashrc") || !f.widget.installed {
		t.Fatalf("install not confirmed (calls %v):\n%s", f.widget.calls, f.view())
	}
	f.press(t, "enter")
	if !strings.Contains(f.view(), "✓ Widget removed from ~/.bashrc") || f.widget.installed {
		t.Fatalf("uninstall not confirmed (calls %v):\n%s", f.widget.calls, f.view())
	}
}

func TestSettingsWidgetOnFishIsNotSupported(t *testing.T) {
	f := newSettingsRoot(t, setupapp.ShellInfo{Name: "fish", Note: "fish is not supported yet — the widget supports bash and zsh"})
	f.press(t, "s")
	if !strings.Contains(f.view(), "not supported yet") {
		t.Fatalf("fish status missing:\n%s", f.view())
	}
	f.press(t, "down", "down", "down", "enter")
	if len(f.widget.calls) != 0 {
		t.Fatalf("widget toggled on fish: %v", f.widget.calls)
	}
	if !strings.Contains(f.view(), "✗") {
		t.Fatalf("no error status on fish toggle:\n%s", f.view())
	}
}

func TestSettingsMotionToggleSaves(t *testing.T) {
	f := newSettingsRoot(t, setupapp.ShellInfo{Name: "bash", Supported: true})
	f.press(t, "s", "down", "down", "down", "down", "enter")
	if f.saved.Motion || f.deps.Config.Motion {
		t.Fatalf("motion still on: saved %+v, running %+v", f.saved, *f.deps.Config)
	}
	if !strings.Contains(f.view(), "✓ Motion off") {
		t.Fatalf("no confirmation:\n%s", f.view())
	}
}

func TestBoardFooterAndHelpListSettings(t *testing.T) {
	f := newSettingsRoot(t, setupapp.ShellInfo{Name: "bash", Supported: true})
	if !strings.Contains(f.view(), "s settings") {
		t.Errorf("board footer lacks the settings hint:\n%s", f.view())
	}
	f.press(t, "?")
	if !strings.Contains(f.view(), "settings") {
		t.Errorf("help overlay lacks settings:\n%s", f.view())
	}
}
