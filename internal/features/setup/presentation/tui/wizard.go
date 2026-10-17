// Package tui is the full-screen setup wizard. It only collects a
// setupapp.SetupInput and hands it to an ApplyFunc; all validation that
// matters and every write live in the application layer.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	"github.com/lesquel/termino/internal/tuikit"
)

// ApplyFunc applies the collected input (setupapp.Service.Apply).
type ApplyFunc func(setupapp.SetupInput) (setupapp.SetupResult, error)

// ThemeLoader loads a theme by name for the live preview.
type ThemeLoader func(name string) (*tuikit.Theme, error)

type step int

const (
	stepMode    step = iota // re-run: edit / add / cancel
	stepWelcome             // first run
	stepName
	stepProject
	stepKey
	stepMethod
	stepAssoc
	stepTheme
	stepWidget
	stepSummary
	stepDone
)

// totalSteps counts the numbered steps, stepName through stepSummary.
const totalSteps = int(stepSummary-stepName) + 1

// modeOptions are the re-run choices, in display order.
var modeOptions = []string{"Edit current setup", "Add another project", "Cancel"}

// methodologies are offered in this order, with a one-line explanation.
var methodologies = []struct {
	kind projectdomain.MethodologyKind
	desc string
}{
	{projectdomain.MethodologyKanban, "Kanban — continuous flow: Backlog → In Progress → Done"},
	{projectdomain.MethodologyScrum, "Scrum — sprints: Backlog → Sprint Backlog → In Progress → Review → Done"},
}

// appliedMsg carries the result of ApplyFunc.
type appliedMsg struct {
	res setupapp.SetupResult
	err error
}

// Model is the setup wizard's Bubble Tea model.
type Model struct {
	state     setupapp.SetupState
	apply     ApplyFunc
	loadTheme ThemeLoader
	chrome    *tuikit.Theme

	step       step
	in         setupapp.SetupInput
	name       textinput.Model
	project    textinput.Model
	key        textinput.Model
	keyTouched bool

	modeCursor   int
	methodCursor int
	themeCursor  int
	assoc        bool
	widget       bool

	problem  string // live validation message for the current step
	applyErr string
	applying bool
	result   setupapp.SetupResult

	cancelled bool
	finished  bool
	width     int
	height    int
}

// New builds the wizard. chrome styles the wizard itself; loadTheme feeds
// the theme preview.
func New(state setupapp.SetupState, apply ApplyFunc, loadTheme ThemeLoader, chrome *tuikit.Theme) Model {
	m := Model{
		state:     state,
		apply:     apply,
		loadTheme: loadTheme,
		chrome:    chrome,
		name:      newInput(40),
		project:   newInput(40),
		key:       newInput(5),
		width:     80,
		height:    24,
	}
	m.step = stepWelcome
	if state.Configured {
		m.step = stepMode
	}
	m.reset(setupapp.ModeCreate)
	return m
}

func newInput(limit int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = limit
	ti.Focus()
	return ti
}

// reset prefills every field for mode.
func (m *Model) reset(mode setupapp.Mode) {
	m.in = m.state.Defaults(mode)
	m.name.SetValue(m.in.UserName)
	m.name.Placeholder = m.state.DefaultUser
	m.project.SetValue(m.in.ProjectName)
	m.key.SetValue(m.in.ProjectKey)
	m.keyTouched = false
	m.methodCursor = 0
	for i, mt := range methodologies {
		if mt.kind == m.in.Methodology {
			m.methodCursor = i
		}
	}
	m.themeCursor = 0
	for i, t := range m.state.Themes {
		if t == m.in.Theme {
			m.themeCursor = i
		}
	}
	m.assoc = m.in.Associate
	m.widget = m.in.InstallWidget && m.state.Shell.Supported
}

// Cancelled reports whether the user left without applying anything.
func (m Model) Cancelled() bool { return m.cancelled }

// Result returns the applied result once the wizard finished.
func (m Model) Result() (setupapp.SetupResult, bool) {
	return m.result, m.step == stepDone
}

// Input returns the input the wizard would apply now.
func (m Model) Input() setupapp.SetupInput {
	in := m.in
	in.UserName = strings.TrimSpace(m.name.Value())
	if in.UserName == "" {
		in.UserName = m.state.DefaultUser
	}
	in.ProjectName = strings.TrimSpace(m.project.Value())
	in.ProjectKey = strings.TrimSpace(m.key.Value())
	in.Methodology = methodologies[m.methodCursor].kind
	in.Associate = m.assoc && m.state.Cwd != ""
	if len(m.state.Themes) > 0 {
		in.Theme = m.state.Themes[m.themeCursor]
	}
	in.InstallWidget = m.widget && m.state.Shell.Supported
	return in
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case appliedMsg:
		m.applying = false
		if msg.err != nil {
			m.applyErr = msg.err.Error()
			return m, nil
		}
		m.result = msg.res
		m.step = stepDone
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		if m.step == stepDone {
			m.finished = true
		} else {
			m.cancelled = true
		}
		return m, tea.Quit
	}
	if m.applying {
		return m, nil
	}
	if m.step == stepDone {
		if k == "enter" || k == "q" || k == "esc" {
			m.finished = true
			return m, tea.Quit
		}
		return m, nil
	}
	if k == "esc" || k == "shift+tab" {
		return m.back()
	}

	switch m.step {
	case stepMode:
		switch k {
		case "up", "k":
			m.modeCursor = (m.modeCursor + len(modeOptions) - 1) % len(modeOptions)
		case "down", "j":
			m.modeCursor = (m.modeCursor + 1) % len(modeOptions)
		case "enter":
			switch m.modeCursor {
			case 0:
				m.reset(setupapp.ModeEdit) // falls back to create without a current project
			case 1:
				m.reset(setupapp.ModeCreate)
			default:
				m.cancelled = true
				return m, tea.Quit
			}
			m.step = stepName
		}
		return m, nil
	case stepWelcome:
		if k == "enter" {
			m.step = stepName
		}
		return m, nil
	case stepName, stepProject:
		if k == "enter" || k == "tab" {
			return m.next()
		}
		var cmd tea.Cmd
		if m.step == stepName {
			m.name, cmd = m.name.Update(msg)
		} else {
			m.project, cmd = m.project.Update(msg)
		}
		m.problem = ""
		return m, cmd
	case stepKey:
		if k == "enter" || k == "tab" {
			return m.next()
		}
		if m.in.Mode == setupapp.ModeEdit {
			return m, nil // read-only
		}
		before := m.key.Value()
		var cmd tea.Cmd
		m.key, cmd = m.key.Update(msg)
		if v := strings.ToUpper(m.key.Value()); v != m.key.Value() {
			m.key.SetValue(v)
		}
		if m.key.Value() != before {
			m.keyTouched = true
		}
		m.problem = keyProblem(m.key.Value(), m.state.ExistingKeys)
		return m, cmd
	case stepMethod:
		m.methodCursor = moveCursor(k, m.methodCursor, len(methodologies))
	case stepTheme:
		m.themeCursor = moveCursor(k, m.themeCursor, len(m.state.Themes))
	case stepAssoc:
		m.assoc = toggle(k, m.assoc)
	case stepWidget:
		if m.state.Shell.Supported {
			m.widget = toggle(k, m.widget)
		}
	case stepSummary:
		if k == "enter" {
			m.applying = true
			m.applyErr = ""
			in, apply := m.Input(), m.apply
			return m, func() tea.Msg {
				res, err := apply(in)
				return appliedMsg{res: res, err: err}
			}
		}
		return m, nil
	}
	if k == "enter" || k == "tab" {
		return m.next()
	}
	return m, nil
}

// next validates the current step and advances.
func (m Model) next() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepName:
		if strings.TrimSpace(m.name.Value()) == "" && m.state.DefaultUser == "" {
			m.problem = "Name can't be empty."
			return m, nil
		}
	case stepProject:
		if strings.TrimSpace(m.project.Value()) == "" {
			m.problem = "Project name can't be empty."
			return m, nil
		}
		if m.in.Mode == setupapp.ModeCreate && !m.keyTouched {
			m.key.SetValue(setupapp.SuggestKey(m.project.Value()))
		}
		m.step = stepKey
		m.problem = ""
		if m.in.Mode == setupapp.ModeCreate {
			m.problem = keyProblem(m.key.Value(), m.state.ExistingKeys)
		}
		return m, nil
	case stepKey:
		if m.in.Mode == setupapp.ModeCreate {
			if p := keyProblem(m.key.Value(), m.state.ExistingKeys); p != "" {
				m.problem = p
				return m, nil
			}
		}
	}
	m.problem = ""
	m.step++
	return m, nil
}

// back returns to the previous step, or cancels from the first screen.
func (m Model) back() (tea.Model, tea.Cmd) {
	m.problem = ""
	m.applyErr = ""
	switch m.step {
	case stepMode, stepWelcome:
		m.cancelled = true
		return m, tea.Quit
	case stepName:
		m.step = stepWelcome
		if m.state.Configured {
			m.step = stepMode
		}
	default:
		m.step--
		if m.step == stepKey && m.in.Mode == setupapp.ModeCreate {
			m.problem = keyProblem(m.key.Value(), m.state.ExistingKeys)
		}
	}
	return m, nil
}

func keyProblem(key string, existing map[string]string) string {
	if err := setupapp.KeyProblem(key, existing); err != nil {
		return capitalize(err.Error()) + "."
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func moveCursor(k string, cur, n int) int {
	if n == 0 {
		return 0
	}
	switch k {
	case "up", "k":
		return (cur + n - 1) % n
	case "down", "j":
		return (cur + 1) % n
	}
	return cur
}

func toggle(k string, v bool) bool {
	switch k {
	case "y":
		return true
	case "n":
		return false
	case "up", "down", "k", "j", "left", "right", "h", "l", " ":
		return !v
	}
	return v
}

// ---------------------------------------------------------------- view

// View implements tea.Model.
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	cw := w - 2
	if cw > 98 {
		cw = 98
	}
	if cw < 20 {
		cw = 20
	}
	t := m.chrome
	accent := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(t.TextMuted)
	para := lipgloss.NewStyle().Width(cw)

	title, help, body, keys := m.content(cw)

	var b strings.Builder
	head := accent.Render("termino setup")
	if m.step >= stepName && m.step <= stepSummary {
		progress := fmt.Sprintf("Step %d of %d", int(m.step-stepName)+1, totalSteps)
		gap := cw - lipgloss.Width(head) - len(progress)
		if gap < 1 {
			gap = 1
		}
		head += strings.Repeat(" ", gap) + muted.Render(progress)
	}
	b.WriteString(head + "\n")
	b.WriteString(muted.Render(strings.Repeat("─", cw)) + "\n\n")
	b.WriteString(accent.Render(title) + "\n")
	if help != "" {
		b.WriteString(para.Foreground(t.TextMuted).Render(help) + "\n")
	}
	b.WriteString("\n" + body + "\n")
	if m.problem != "" {
		b.WriteString("\n" + para.Foreground(t.Danger).Render("✗ "+m.problem) + "\n")
	}
	if m.applyErr != "" {
		b.WriteString("\n" + para.Foreground(t.Danger).Render("✗ Setup failed: "+m.applyErr) + "\n")
	}
	if m.applying {
		b.WriteString("\n" + muted.Render("Applying…") + "\n")
	}
	b.WriteString("\n" + para.Foreground(t.TextMuted).Render(keys))
	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}

const (
	keysText    = "enter next · esc back · ctrl+c quit"
	keysSelect  = "↑/↓ choose · enter next · esc back · ctrl+c quit"
	keysYesNo   = "y/n or ↑/↓ choose · enter next · esc back · ctrl+c quit"
	keysSummary = "enter confirm · esc go back · ctrl+c quit"
)

// content returns the title, help line, body and key footer of the step.
func (m Model) content(cw int) (string, string, string, string) {
	t := m.chrome
	text := lipgloss.NewStyle().Foreground(t.Text)
	para := lipgloss.NewStyle().Width(cw).Foreground(t.Text)
	edit := m.in.Mode == setupapp.ModeEdit

	switch m.step {
	case stepMode:
		cur := "no current project"
		if p := m.state.Current; p != nil {
			cur = fmt.Sprintf("current project %s (%s)", p.Name, p.Key)
		}
		return "Termino is already set up",
			fmt.Sprintf("User %s · %s. Nothing changes until you confirm the summary.", m.state.UserName, cur),
			m.list(modeOptions, m.modeCursor), keysSelect
	case stepWelcome:
		return "Welcome to Termino",
			"",
			para.Render("A task board that lives in your terminal. This wizard sets your name, a first project, a theme and an optional shell widget. It takes under a minute, and you can go back at any step."),
			"enter start · esc quit"
	case stepName:
		help := "Used to attribute activity and to assign tasks to you."
		if m.state.DefaultUser != "" {
			help += fmt.Sprintf(" Leave empty to use $USER (%s).", m.state.DefaultUser)
		}
		return "Your name", help, m.name.View(), keysText
	case stepProject:
		help := "The board you'll work on. Defaults to this directory's name."
		if edit {
			help = "Rename the current project."
		}
		return "Project name", help, m.project.View(), keysText
	case stepKey:
		if edit {
			return "Project key (read-only)",
				"Task IDs already use it, so it can't change. Create another project for a new key.",
				text.Render(fmt.Sprintf("%s  (tasks are %s-1, %s-2, …)", m.key.Value(), m.key.Value(), m.key.Value())), keysText
		}
		example := m.key.Value()
		if m.problem != "" || example == "" {
			example = "KEY"
		}
		return "Project key",
			fmt.Sprintf("2-5 uppercase letters, suggested from the name. Task IDs look like %s-1.", example),
			m.key.View(), keysText
	case stepMethod:
		help := "How the board's columns are laid out."
		if edit {
			help = "Changing it keeps the project's existing columns."
		}
		opts := make([]string, len(methodologies))
		for i, mt := range methodologies {
			opts[i] = mt.desc
		}
		return "Methodology", help, m.list(opts, m.methodCursor), keysSelect
	case stepAssoc:
		if m.state.Cwd == "" {
			return "Associate this directory?", "The working directory is unknown, so nothing will be associated.", "", keysText
		}
		no := "No"
		if edit && m.state.Current != nil && m.state.Current.AssociatedPath != "" {
			no = "No — keep " + m.state.Current.AssociatedPath
		}
		return "Associate this directory?",
			"When your shell is inside it, the widget picks this project automatically.",
			para.Render(m.state.Cwd) + "\n\n" + m.yesNo("Yes", no, m.assoc), keysYesNo
	case stepTheme:
		return "Theme", "The preview follows the highlighted theme.", m.themeBody(cw), keysSelect
	case stepWidget:
		sh := m.state.Shell
		if !sh.Supported {
			return "Shell widget", "Detected shell: " + sh.Name,
				para.Render("Skipped: " + sh.Note + ". Run `termino widget install --shell bash` later if you switch."), keysText
		}
		help := fmt.Sprintf("Detected shell: %s. Prints a one-line task summary in each new shell (~/.%src).", sh.Name, sh.Name)
		if m.state.WidgetInstalled {
			help += " Already installed."
		}
		return "Install the shell widget?", help, m.yesNo("Yes", "No", m.widget), keysYesNo
	case stepSummary:
		return "Review your setup", "Nothing has been written yet.", m.summary(cw), keysSummary
	default:
		return "You're all set", "", m.doneBody(cw), "enter exit"
	}
}

// list renders options with a text marker so it reads under NO_COLOR.
func (m Model) list(opts []string, cursor int) string {
	sel := lipgloss.NewStyle().Foreground(m.chrome.Accent).Bold(true)
	plain := lipgloss.NewStyle().Foreground(m.chrome.Text)
	lines := make([]string, len(opts))
	for i, o := range opts {
		if i == cursor {
			lines[i] = sel.Render("› " + o)
		} else {
			lines[i] = plain.Render("  " + o)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) yesNo(yes, no string, v bool) string {
	cur := 1
	if v {
		cur = 0
	}
	return m.list([]string{yes, no}, cur)
}

func (m Model) themeBody(cw int) string {
	names := make([]string, len(m.state.Themes))
	listW := 0
	for i, n := range m.state.Themes {
		names[i] = n
		if m.state.Configured && n == m.state.Theme {
			names[i] += " (current)"
		}
		if l := len(names[i]) + 2; l > listW {
			listW = l
		}
	}
	list := m.list(names, m.themeCursor)
	if len(m.state.Themes) == 0 {
		return list
	}
	name := m.state.Themes[m.themeCursor]
	var preview string
	th, err := m.loadTheme(name)
	if err != nil {
		preview = "preview unavailable: " + err.Error()
	} else {
		pw := cw
		side := cw >= 60+listW+3
		if side {
			pw = cw - listW - 3
		}
		preview = RenderPreview(th, strings.TrimSpace(m.project.Value()), m.key.Value(), pw)
		if side {
			return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listW+3).Render(list), preview)
		}
	}
	return list + "\n\n" + preview
}

func (m Model) summary(cw int) string {
	in := m.Input()
	label := lipgloss.NewStyle().Foreground(m.chrome.TextMuted).Width(13)
	val := lipgloss.NewStyle().Foreground(m.chrome.Text).Width(cw - 13)
	action := "Create a new project"
	if in.Mode == setupapp.ModeEdit {
		action = "Update the current project (no new project)"
	}
	dir := "not associated"
	switch {
	case in.Associate:
		dir = m.state.Cwd
	case in.Mode == setupapp.ModeEdit && m.state.Current != nil && m.state.Current.AssociatedPath != "":
		dir = "keep " + m.state.Current.AssociatedPath
	}
	widget := "no"
	switch {
	case !m.state.Shell.Supported:
		widget = "skipped — " + m.state.Shell.Note
	case in.InstallWidget:
		widget = "install for " + m.state.Shell.Name
	}
	key := in.ProjectKey
	if in.Mode == setupapp.ModeEdit {
		key += " (unchanged)"
	}
	rows := [][2]string{
		{"Action", action},
		{"Name", in.UserName},
		{"Project", in.ProjectName},
		{"Key", key},
		{"Methodology", string(in.Methodology)},
		{"Directory", dir},
		{"Theme", in.Theme},
		{"Widget", widget},
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, label.Render(r[0]), val.Render(r[1]))
	}
	return strings.Join(lines, "\n")
}

func (m Model) doneBody(cw int) string {
	ok := lipgloss.NewStyle().Foreground(m.chrome.Success)
	text := lipgloss.NewStyle().Foreground(m.chrome.Text).Width(cw)
	verb := "created"
	if !m.result.Created {
		verb = "updated"
	}
	lines := []string{ok.Render(fmt.Sprintf("✓ Project %s: %s (%s)", verb, m.result.Project.Name, m.result.Project.Key))}
	if m.result.WidgetInstalled {
		lines = append(lines, ok.Render("✓ Shell widget installed — open a new shell to see it"))
	} else if m.result.WidgetNote != "" {
		lines = append(lines, text.Render("  Widget skipped: "+m.result.WidgetNote))
	}
	lines = append(lines, "", text.Render("Next steps:"))
	for _, s := range setupapp.NextSteps() {
		lines = append(lines, text.Render("  "+s))
	}
	return strings.Join(lines, "\n")
}
