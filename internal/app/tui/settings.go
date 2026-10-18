package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	setuptui "github.com/lesquel/termino/internal/features/setup/presentation/tui"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/tuikit"
)

// WidgetToggler installs and removes the shell widget block; it is the same
// use case `termino widget install/uninstall` runs.
type WidgetToggler interface {
	Status() (setupapp.WidgetStatus, error)
	Install() (string, error)
	Uninstall() (string, error)
}

// ShareToggler shares the current project through its git repository or
// stops sharing it; it is the same use case as `termino share init/off`.
type ShareToggler interface {
	// ShareRoot returns the repository the project is shared from, or "".
	ShareRoot(projectID string) (string, error)
	// EnableShare shares the project from the git repository of the working
	// directory and returns that repository's root.
	EnableShare(projectID string) (string, error)
	DisableShare(projectID string) error
}

// SettingsDeps feeds the settings screen. Zero values fall back to the
// built-in themes, the theme loader and config.Load/Save; a nil Widget
// shows the widget row as unavailable.
type SettingsDeps struct {
	Themes     []string
	LoadTheme  func(name string) (*tuikit.Theme, error)
	SaveConfig func(mutate func(*config.Config)) error
	Widget     WidgetToggler
	Share      ShareToggler // nil shows the share row as unavailable
	Home       string       // shortens paths under it to "~"
}

// SettingsClosedMsg asks the root to leave the settings screen.
type SettingsClosedMsg struct{}

// MotionChangedMsg tells the root that animations were switched on or off.
type MotionChangedMsg struct{ On bool }

// SettingsModel is the settings screen: theme (with live preview), shell
// widget and motion. It owns every key while open.
type SettingsModel struct {
	theme     *tuikit.Theme // the running theme, shared with every screen
	deps      SettingsDeps
	project   string
	key       string
	projectID string

	current string // active theme name
	motion  bool
	widget  setupapp.WidgetStatus
	wErr    string
	share   string // share root, "" when not shared
	sErr    string

	cursor    int // themes first, then the widget row, motion, then sharing
	status    string
	statusErr bool
	width     int
	height    int
}

// NewSettingsModel builds the settings screen for cfg. theme is the running
// theme: applying a theme overwrites it in place so every screen follows.
func NewSettingsModel(theme *tuikit.Theme, cfg *config.Config, deps SettingsDeps, project, key string) SettingsModel {
	if len(deps.Themes) == 0 {
		deps.Themes = tuikit.Available(userThemesDir())
	}
	if deps.LoadTheme == nil {
		deps.LoadTheme = Load
	}
	if deps.SaveConfig == nil {
		deps.SaveConfig = saveConfig
	}
	current := cfg.Theme
	if current == "" {
		current = "default"
	}
	m := SettingsModel{
		theme:   theme,
		deps:    deps,
		project: project,
		key:     key,
		current: current,
		motion:  cfg.Motion,
	}
	for i, n := range deps.Themes {
		if n == current {
			m.cursor = i
		}
	}
	m.refreshWidget()
	return m
}

// forProject returns the screen bound to the project the share row toggles.
func (m SettingsModel) forProject(projectID string) SettingsModel {
	m.projectID = projectID
	m.refreshShare()
	return m
}

func (m *SettingsModel) refreshShare() {
	m.sErr, m.share = "", ""
	if m.deps.Share == nil || m.projectID == "" {
		m.sErr = "unavailable"
		return
	}
	root, err := m.deps.Share.ShareRoot(m.projectID)
	if err != nil {
		m.sErr = err.Error()
	}
	m.share = root
}

// saveConfig applies mutate to the config on disk, so settings changed
// elsewhere (e.g. the current project) are kept.
func saveConfig(mutate func(*config.Config)) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	mutate(cfg)
	return config.Save(cfg)
}

func (m *SettingsModel) refreshWidget() {
	m.wErr = ""
	if m.deps.Widget == nil {
		m.widget = setupapp.WidgetStatus{}
		m.wErr = "unavailable"
		return
	}
	st, err := m.deps.Widget.Status()
	if err != nil {
		m.wErr = err.Error()
	}
	m.widget = st
}

func (m SettingsModel) widgetRow() int { return len(m.deps.Themes) }
func (m SettingsModel) motionRow() int { return len(m.deps.Themes) + 1 }
func (m SettingsModel) shareRow() int  { return len(m.deps.Themes) + 2 }

// Init implements tea.Model.
func (m SettingsModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return SettingsClosedMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.shareRow() {
				m.cursor++
			}
		case "enter", " ":
			return m.activate()
		}
	}
	return m, nil
}

// activate applies the highlighted theme or flips the highlighted toggle.
func (m SettingsModel) activate() (tea.Model, tea.Cmd) {
	switch {
	case m.cursor < m.widgetRow():
		m.applyTheme(m.deps.Themes[m.cursor])
	case m.cursor == m.widgetRow():
		m.toggleWidget()
	case m.cursor == m.shareRow():
		m.toggleShare()
	default:
		on := !m.motion
		if err := m.deps.SaveConfig(func(c *config.Config) { c.Motion = on }); err != nil {
			m.fail("save config: " + err.Error())
			return m, nil
		}
		m.motion = on
		m.ok("Motion " + onOff(on))
		return m, func() tea.Msg { return MotionChangedMsg{On: on} }
	}
	return m, nil
}

func (m *SettingsModel) applyTheme(name string) {
	th, err := m.deps.LoadTheme(name)
	if err != nil {
		m.fail(err.Error())
		return
	}
	*m.theme = *th
	m.current = name
	if err := m.deps.SaveConfig(func(c *config.Config) { c.Theme = name }); err != nil {
		m.fail("theme applied but not saved: " + err.Error())
		return
	}
	m.ok("Theme set to " + name)
}

func (m *SettingsModel) toggleWidget() {
	if m.deps.Widget == nil {
		m.fail("shell widget unavailable")
		return
	}
	if !m.widget.Installed && !m.widget.Shell.Supported {
		m.fail(m.widget.Shell.Note)
		return
	}
	if m.widget.Installed {
		path, err := m.deps.Widget.Uninstall()
		if err != nil {
			m.fail(err.Error())
		} else {
			m.ok("Widget removed from " + m.short(path))
		}
	} else {
		path, err := m.deps.Widget.Install()
		if err != nil {
			m.fail(err.Error())
		} else {
			m.ok("Widget installed in " + m.short(path) + " — open a new shell to see it")
		}
	}
	m.refreshWidget()
}

func (m *SettingsModel) toggleShare() {
	if m.deps.Share == nil || m.projectID == "" {
		m.fail("sharing unavailable")
		return
	}
	if m.share != "" {
		if err := m.deps.Share.DisableShare(m.projectID); err != nil {
			m.fail(err.Error())
		} else {
			m.ok("Sharing off — the files stay in " + m.short(m.share))
		}
	} else {
		root, err := m.deps.Share.EnableShare(m.projectID)
		if err != nil {
			m.fail(err.Error())
		} else {
			m.ok("Sharing through " + m.short(root) + "/.termino — commit it to share the board")
		}
	}
	m.refreshShare()
}

func (m *SettingsModel) ok(s string)   { m.status, m.statusErr = "✓ "+s, false }
func (m *SettingsModel) fail(s string) { m.status, m.statusErr = "✗ "+s, true }

// short replaces the home directory prefix of path with "~".
func (m SettingsModel) short(path string) string {
	if m.deps.Home != "" && strings.HasPrefix(path, m.deps.Home+string(os.PathSeparator)) {
		return "~" + path[len(m.deps.Home):]
	}
	return path
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// onOffLabel is onOff capitalised for the settings rows.
func onOffLabel(on bool) string {
	if on {
		return "On"
	}
	return "Off"
}

// frame is the settings dialog without its body. It is wide (up to the
// terminal width) so the theme preview fits beside the list.
func (m SettingsModel) frame() tuikit.Dialog {
	sub := ""
	if m.project != "" {
		sub = "Project: " + m.project
		if m.key != "" {
			sub += " (" + m.key + ")"
		}
	}
	d := tuikit.Dialog{
		Title:    "Settings",
		Subtitle: sub,
		Badge:    "esc back",
		Footer:   "↑/↓ move · enter apply/toggle · esc back",
		Width:    90,
	}
	if m.width > 0 {
		d.MaxWidth = m.width - 2
	}
	return d
}

// View implements tea.Model. Settings is a dialog; the root draws it over
// the board.
func (m SettingsModel) View() string {
	t := m.theme
	d := m.frame()
	inner := d.Inner(m.width)
	accent := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(t.TextMuted)
	text := lipgloss.NewStyle().Foreground(t.Text)
	sel := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	row := func(i int, s string) string {
		if i == m.cursor {
			return sel.Render("› " + s)
		}
		return text.Render("  " + s)
	}

	var list []string
	list = append(list, accent.Render("Theme"))
	listW := 0
	for i, n := range m.deps.Themes {
		label := n
		if n == m.current {
			label += " ●"
		}
		listW = max(listW, ansi.StringWidth(label)+2)
		list = append(list, row(i, label))
	}

	// Preview the highlighted theme, or the active one off the theme list.
	previewName := m.current
	if m.cursor < m.widgetRow() {
		previewName = m.deps.Themes[m.cursor]
	}
	var preview string
	if th, err := m.deps.LoadTheme(previewName); err == nil {
		pw := inner - listW - 5
		if pw >= 44 {
			preview = setuptui.RenderPreview(th, m.project, m.key, pw)
		}
	}
	themes := strings.Join(list, "\n")
	if preview != "" {
		themes = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(listW+5).Render(themes), "\n"+preview)
	}

	widgetState := onOffLabel(m.widget.Installed)
	widgetInfo := ""
	switch {
	case m.wErr != "":
		widgetState, widgetInfo = "—", m.wErr
	case m.widget.Installed:
		widgetInfo = m.widget.Shell.Name + " · " + m.short(m.widget.InstalledPath)
	case m.widget.Shell.Supported:
		widgetInfo = m.widget.Shell.Name + " · " + m.short(m.widget.RCPath)
	default:
		widgetState, widgetInfo = "—", m.widget.Shell.Name+": not supported yet"
	}
	shareState, shareInfo := onOffLabel(m.share != ""), "enter shares from this git repository"
	switch {
	case m.sErr != "":
		shareState, shareInfo = "—", m.sErr
	case m.share != "":
		shareInfo = m.short(m.share)
	}
	motionInfo := ""
	if m.motion && os.Getenv("NO_COLOR") != "" {
		motionInfo = "off while NO_COLOR is set"
	}

	label := func(s string) string { return fmt.Sprintf("%-14s", s) }
	body := strings.Split(themes, "\n")
	widgetLine := len(body) + 1
	body = append(body,
		"",
		row(m.widgetRow(), label("Shell widget")+widgetState)+"  "+muted.Render(widgetInfo),
		row(m.motionRow(), label("Motion")+onOffLabel(m.motion))+"  "+muted.Render(motionInfo),
		row(m.shareRow(), label("Share via repo")+shareState)+"  "+muted.Render(shareInfo),
	)
	if m.status != "" {
		body = append(body, "")
		st := lipgloss.NewStyle().Foreground(t.Success)
		if m.statusErr {
			st = lipgloss.NewStyle().Foreground(t.Danger)
		}
		body = append(body, st.Render(m.status))
	}
	d.Body = body

	// Keep the highlighted row in view on short terminals.
	cursorLine := 1 + m.cursor // themes follow the "Theme" heading
	if m.cursor >= m.widgetRow() {
		cursorLine = widgetLine + m.cursor - m.widgetRow()
	}
	if room := d.BodyRoom(m.width, m.height); m.height > 0 && room > 0 && cursorLine >= room {
		d.Scroll = cursorLine - room + 1
	}
	return d.Render(t, m.width, m.height)
}
