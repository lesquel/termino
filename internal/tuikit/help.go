package tuikit

// HelpEntry is one row of the help dialog. Keys are the key strings the
// handlers compare against (tea.KeyMsg.String()); Label is how they are
// shown. An entry without keys continues the previous description.
type HelpEntry struct {
	Keys  []string
	Label string
	Desc  string
}

// HelpSection groups the keys of one screen or context.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// Covers reports whether the section documents key k.
func (s HelpSection) Covers(k string) bool {
	for _, e := range s.Entries {
		for _, have := range e.Keys {
			if have == k {
				return true
			}
		}
	}
	return false
}

// Help section titles, so tests can check one context at a time.
const (
	HelpNavigate = "Board · navigate"
	HelpTasks    = "Board · tasks"
	HelpViews    = "Board · views"
	HelpPickers  = "Pickers & delete prompt"
	HelpDetail   = "Task detail"
	HelpForm     = "New task"
	HelpSelector = "Project selector"
	HelpSettings = "Settings"
	HelpGeneral  = "General"
)

var vertical = []string{"up", "k", "down", "j"}

// HelpSections lists every key the TUI handles, grouped by context. Tests
// in each presentation package compare it with the keys their Update
// functions switch on, so a new key cannot ship undocumented.
func HelpSections() []HelpSection {
	return []HelpSection{
		{HelpNavigate, []HelpEntry{
			{[]string{"up", "k"}, "↑/k", "previous card"},
			{[]string{"down", "j"}, "↓/j", "next card"},
			{[]string{"left", "h"}, "←/h", "previous column"},
			{[]string{"right", "l"}, "→/l", "next column"},
		}},
		{HelpTasks, []HelpEntry{
			{[]string{"enter"}, "enter", "open task detail"},
			{[]string{"n"}, "n", "new task in this column"},
			{[]string{"m"}, "m", "move: pick a column"},
			{[]string{"H", "shift+left"}, "H/shift+←", "move to the left column"},
			{[]string{"L", "shift+right"}, "L/shift+→", "move to the right column"},
			{[]string{"a"}, "a", "assign: pick a member"},
			{[]string{"d"}, "d", "delete (asks y/N)"},
		}},
		{HelpViews, []HelpEntry{
			{[]string{"/"}, "/", "filter cards by text or field:"},
			{nil, "", "assignee: label: priority: agent:"},
			{[]string{"esc"}, "esc", "clear the filter and status"},
			{[]string{"p"}, "p", "project selector"},
			{[]string{"s"}, "s", "settings: theme, widget, motion"},
			{[]string{"?"}, "?", "this help"},
		}},
		{HelpPickers, []HelpEntry{
			{vertical, "↑↓/jk", "choose"},
			{[]string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, "1-9", "pick by number"},
			{[]string{"enter"}, "enter", "confirm"},
			{[]string{"esc", "q"}, "esc/q", "cancel"},
			{[]string{"y", "Y"}, "y", "delete; any other key keeps the task"},
		}},
		{HelpDetail, []HelpEntry{
			{vertical, "↑↓/jk", "scroll"},
			{[]string{"e"}, "e", "edit title and description"},
			{[]string{"esc", "enter"}, "esc/enter", "close (esc also cancels an edit)"},
			{[]string{"tab"}, "tab", "edit: switch field"},
			{[]string{"ctrl+s"}, "ctrl+s", "edit: save (enter saves the title)"},
		}},
		{HelpForm, []HelpEntry{
			{[]string{"tab", "down"}, "tab/↓", "next field"},
			{[]string{"shift+tab", "up"}, "shift+tab/↑", "previous field"},
			{[]string{"left", "h", "-", "right", "l", "+", " "}, "←/→ h/l", "priority down / up (also - + space)"},
			{[]string{"enter"}, "enter", "create the task"},
			{[]string{"esc"}, "esc", "cancel"},
		}},
		{HelpSelector, []HelpEntry{
			{vertical, "↑↓/jk", "choose a project"},
			{[]string{"enter"}, "enter", "open it"},
			{[]string{"/"}, "/", "filter projects"},
			{[]string{"q"}, "q", "back to the board"},
		}},
		{HelpSettings, []HelpEntry{
			{vertical, "↑↓/jk", "move"},
			{[]string{"enter", " "}, "enter/space", "apply theme / toggle"},
			{[]string{"esc"}, "esc", "back to the board"},
		}},
		{HelpGeneral, []HelpEntry{
			{[]string{"q"}, "q", "quit"},
			{[]string{"ctrl+c"}, "ctrl+c", "quit from anywhere"},
			{[]string{"?"}, "?", "help (any other key closes it)"},
			{vertical, "↑↓/jk", "scroll this help"},
		}},
	}
}

// HelpSectionNamed returns the section with the given title.
func HelpSectionNamed(title string) (HelpSection, bool) {
	for _, s := range HelpSections() {
		if s.Title == title {
			return s, true
		}
	}
	return HelpSection{}, false
}
