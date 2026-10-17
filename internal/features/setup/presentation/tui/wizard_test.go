package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	"github.com/lesquel/termino/internal/tuikit"
)

func loader(name string) (*tuikit.Theme, error) { return tuikit.Load(name, "") }

func firstRunState() setupapp.SetupState {
	return setupapp.SetupState{
		ExistingKeys: map[string]string{"OLD": "Old"},
		Themes:       []string{"default", "spooky"},
		Cwd:          "/work/web-app",
		DefaultUser:  "leo",
		Shell:        setupapp.ShellInfo{Name: "bash", Supported: true},
	}
}

type recorder struct {
	got   *setupapp.SetupInput
	err   error
	calls int
}

func (r *recorder) apply(in setupapp.SetupInput) (setupapp.SetupResult, error) {
	r.calls++
	r.got = &in
	if r.err != nil {
		return setupapp.SetupResult{}, r.err
	}
	return setupapp.SetupResult{
		Project: projectdomain.Project{Name: in.ProjectName, Key: in.ProjectKey},
		Created: in.Mode == setupapp.ModeCreate,
	}, nil
}

func newModel(t *testing.T, st setupapp.SetupState, r *recorder) Model {
	t.Helper()
	chrome, err := tuikit.Load("default", "")
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, r.apply, loader, chrome)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m2.(Model)
}

// press sends keys one at a time. When a key starts applying, the returned
// command is run so the apply message reaches the model; other commands
// (cursor blink ticks) are ignored.
func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		case "ctrl+c":
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, cmd := m.Update(msg)
		m = next.(Model)
		if cmd != nil && m.applying {
			if res, ok := cmd().(appliedMsg); ok {
				next, _ = m.Update(res)
				m = next.(Model)
			}
		}
	}
	return m
}

func view(m Model) string { return ansi.Strip(m.View()) }

func TestFirstRunHappyPathWithDefaults(t *testing.T) {
	r := &recorder{}
	m := newModel(t, firstRunState(), r)
	if !strings.Contains(view(m), "Welcome to Termino") {
		t.Fatalf("first screen:\n%s", view(m))
	}
	m = press(m, "enter")
	v := view(m)
	if !strings.Contains(v, "Step 1 of 8") || !strings.Contains(v, "leo") || !strings.Contains(v, "esc back") {
		t.Fatalf("name step:\n%s", v)
	}
	m = press(m, "enter") // name leo
	if v := view(m); !strings.Contains(v, "web-app") || !strings.Contains(v, "Step 2 of 8") {
		t.Fatalf("project step:\n%s", v)
	}
	m = press(m, "enter") // project web-app
	if got := m.key.Value(); got != "WA" {
		t.Fatalf("suggested key = %q, want WA", got)
	}
	m = press(m, "enter", "enter", "enter", "enter", "enter") // key, methodology, associate, theme, widget
	v = view(m)
	if !strings.Contains(v, "Review your setup") || !strings.Contains(v, "Step 8 of 8") || !strings.Contains(v, "enter confirm") {
		t.Fatalf("summary:\n%s", v)
	}
	m = press(m, "enter")
	if r.calls != 1 {
		t.Fatalf("apply calls = %d", r.calls)
	}
	want := setupapp.SetupInput{Mode: setupapp.ModeCreate, UserName: "leo", ProjectName: "web-app", ProjectKey: "WA",
		Methodology: projectdomain.MethodologyKanban, Associate: true, Theme: "default"}
	if *r.got != want {
		t.Fatalf("applied %+v\nwant     %+v", *r.got, want)
	}
	v = view(m)
	for _, s := range []string{"Project created: web-app (WA)", "termino board", "n ", "? ", `termino add "`} {
		if !strings.Contains(v, s) {
			t.Errorf("done screen missing %q:\n%s", s, v)
		}
	}
	if _, done := m.Result(); !done || m.Cancelled() {
		t.Fatal("wizard should report done, not cancelled")
	}
}

func TestKeySuggestionAndLiveValidation(t *testing.T) {
	m := newModel(t, firstRunState(), &recorder{})
	m = press(m, "enter", "enter", "ctrl+u")
	m = press(m, strings.Split("Web App", "")...)
	m = press(m, "enter")
	if m.key.Value() != "WA" {
		t.Fatalf("key for Web App = %q", m.key.Value())
	}
	m = press(m, "backspace", "1") // "W1"
	v := view(m)
	if !strings.Contains(v, "must be 2-5 uppercase letters") {
		t.Fatalf("live error not shown:\n%s", v)
	}
	if m = press(m, "enter"); m.step != stepKey {
		t.Fatal("advanced past an invalid key")
	}
	m = press(m, "backspace", "o", "l", "d") // lowercase is upper-cased: WOLD
	if m.key.Value() != "WOLD" || m.problem != "" {
		t.Fatalf("key = %q problem = %q", m.key.Value(), m.problem)
	}
	m = press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "o", "l", "d")
	if !strings.Contains(view(m), "already used by project") {
		t.Fatalf("duplicate key not flagged:\n%s", view(m))
	}
	// A key the user typed is kept when going back and changing the name.
	m = press(m, "ctrl+u", "z", "z", "esc", "x", "enter")
	if m.key.Value() != "ZZ" {
		t.Fatalf("typed key overwritten: %q", m.key.Value())
	}
}

func TestBackNavigationAndCancel(t *testing.T) {
	m := newModel(t, firstRunState(), &recorder{})
	m = press(m, "enter", "enter", "enter", "enter") // at methodology
	if m.step != stepMethod {
		t.Fatalf("step = %v", m.step)
	}
	m = press(m, "shift+tab", "esc")
	if m.step != stepProject {
		t.Fatalf("after two backs step = %v, want project", m.step)
	}
	m = press(m, "esc", "esc")
	if m.step != stepWelcome {
		t.Fatalf("step = %v, want welcome", m.step)
	}
	m = press(m, "esc")
	if !m.Cancelled() {
		t.Fatal("esc on the first screen should cancel")
	}
}

func TestEmptyProjectNameBlocks(t *testing.T) {
	m := newModel(t, firstRunState(), &recorder{})
	m = press(m, "enter", "enter", "ctrl+u", "enter")
	if m.step != stepProject || !strings.Contains(view(m), "can't be empty") {
		t.Fatalf("empty project accepted:\n%s", view(m))
	}
}

func TestChoicesFlowIntoInput(t *testing.T) {
	r := &recorder{}
	m := newModel(t, firstRunState(), r)
	m = press(m, "enter", "enter", "enter", "enter", "down", "enter", "n", "enter")
	v := view(m)
	if !strings.Contains(v, "spooky") || !strings.Contains(v, "termino · web-app (WA)") || !strings.Contains(v, "Backlog") {
		t.Fatalf("theme step without preview:\n%s", v)
	}
	m = press(m, "down", "enter", "y", "enter", "enter")
	in := r.got
	if in == nil || in.Methodology != projectdomain.MethodologyScrum || in.Associate || in.Theme != "spooky" || !in.InstallWidget {
		t.Fatalf("applied %+v", in)
	}
}

func TestThemePreviewFollowsCursor(t *testing.T) {
	m := newModel(t, firstRunState(), &recorder{})
	m = press(m, "enter", "enter", "enter", "enter", "enter", "enter")
	if m.step != stepTheme {
		t.Fatalf("step = %v", m.step)
	}
	before := m.View()
	after := press(m, "down").View()
	if before == after {
		t.Fatal("preview did not change with the highlighted theme")
	}
}

func TestUnsupportedShellSkipsWidget(t *testing.T) {
	st := firstRunState()
	st.Shell = setupapp.ShellInfo{Name: "fish", Note: "fish is not supported yet"}
	r := &recorder{}
	m := newModel(t, st, r)
	m = press(m, "enter", "enter", "enter", "enter", "enter", "enter", "enter")
	if !strings.Contains(view(m), "fish is not supported yet") {
		t.Fatalf("widget step:\n%s", view(m))
	}
	m = press(m, "y", "enter", "enter")
	if r.got == nil || r.got.InstallWidget {
		t.Fatalf("widget requested on fish: %+v", r.got)
	}
}

func TestApplyErrorStaysOnSummary(t *testing.T) {
	r := &recorder{err: errors.New("boom")}
	m := newModel(t, firstRunState(), r)
	m = press(m, "enter", "enter", "enter", "enter", "enter", "enter", "enter", "enter", "enter")
	if m.step != stepSummary || !strings.Contains(view(m), "Setup failed: boom") {
		t.Fatalf("step = %v\n%s", m.step, view(m))
	}
}

func configuredState() setupapp.SetupState {
	st := firstRunState()
	st.Configured = true
	st.UserName = "Ana"
	st.Theme = "spooky"
	st.Current = &projectdomain.Project{ID: "p1", Name: "Web", Key: "WEB", Methodology: projectdomain.MethodologyKanban, AssociatedPath: "/elsewhere"}
	st.ExistingKeys = map[string]string{"WEB": "Web"}
	return st
}

func TestRerunEditPrefillsAndKeepsKey(t *testing.T) {
	r := &recorder{}
	m := newModel(t, configuredState(), r)
	v := view(m)
	for _, s := range []string{"already set up", "Edit current setup", "Add another project", "Cancel"} {
		if !strings.Contains(v, s) {
			t.Fatalf("mode screen missing %q:\n%s", s, v)
		}
	}
	m = press(m, "enter") // edit
	if m.name.Value() != "Ana" || m.project.Value() != "Web" {
		t.Fatalf("edit prefill name=%q project=%q", m.name.Value(), m.project.Value())
	}
	m = press(m, "enter", "x", "enter") // project "Webx"
	if !strings.Contains(view(m), "read-only") {
		t.Fatalf("key step in edit mode:\n%s", view(m))
	}
	m = press(m, "Q", "enter", "enter") // typing is ignored; methodology
	if !strings.Contains(view(m), "keep /elsewhere") {
		t.Fatalf("assoc step should offer keeping the current path:\n%s", view(m))
	}
	m = press(m, "enter", "enter", "enter")
	if !strings.Contains(view(m), "no new project") {
		t.Fatalf("summary should say no new project:\n%s", view(m))
	}
	m = press(m, "enter")
	in := r.got
	if in.Mode != setupapp.ModeEdit || in.ProjectID != "p1" || in.ProjectKey != "WEB" || in.ProjectName != "Webx" || in.Theme != "spooky" || in.Associate {
		t.Fatalf("edit applied %+v", in)
	}
	if !strings.Contains(view(m), "Project updated: Webx (WEB)") {
		t.Fatalf("done:\n%s", view(m))
	}
}

func TestRerunAddAndCancel(t *testing.T) {
	r := &recorder{}
	m := press(newModel(t, configuredState(), r), "down", "enter")
	if m.in.Mode != setupapp.ModeCreate || m.name.Value() != "Ana" {
		t.Fatalf("add mode = %v name = %q", m.in.Mode, m.name.Value())
	}
	m = press(newModel(t, configuredState(), r), "down", "down", "enter")
	if !m.Cancelled() || r.calls != 0 {
		t.Fatal("cancel should quit without applying")
	}
}

func TestFitsNarrowTerminals(t *testing.T) {
	for _, w := range []int{80, 60, 40} {
		m := newModel(t, firstRunState(), &recorder{})
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = press(next.(Model), "enter", "enter", "enter", "enter", "enter", "enter")
		v := m.View()
		for _, line := range strings.Split(v, "\n") {
			if lw := ansi.StringWidth(line); lw > w {
				t.Fatalf("width %d: line is %d wide: %q", w, lw, ansi.Strip(line))
			}
		}
		if w == 80 {
			if h := strings.Count(v, "\n") + 1; h > 24 {
				t.Fatalf("80x24 theme step is %d lines tall", h)
			}
		}
	}
}

// When setup runs on the way to the board, the done screen says enter opens
// the board instead of "exit".
func TestDoneHintWhenOpeningTheBoard(t *testing.T) {
	m := newModel(t, firstRunState(), &recorder{}).OpeningBoard()
	m = press(m, "enter", "enter", "enter", "enter", "enter", "enter", "enter", "enter", "enter")
	v := view(m)
	if !strings.Contains(v, "enter open the board") || strings.Contains(v, "enter exit") {
		t.Fatalf("done screen:\n%s", v)
	}
	if strings.Contains(v, "termino board ") {
		t.Errorf("done screen still tells the user to run termino board:\n%s", v)
	}
}
