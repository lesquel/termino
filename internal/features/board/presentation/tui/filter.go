package tui

import (
	"strings"

	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

// Filter narrows the cards shown on the board (PRD 5.4, 5.9). It is parsed
// from a query such as "login assignee:bob label:bug priority:high agent:kiro";
// every non-empty predicate must match. Matching is case-insensitive.
type Filter struct {
	Text     string
	Assignee string
	Label    string
	Priority string
	Agent    string
}

// ParseFilter turns a query string into a Filter. Unknown "key:value" tokens
// are treated as free text.
func ParseFilter(query string) Filter {
	var f Filter
	var text []string
	for _, tok := range strings.Fields(query) {
		key, value, found := strings.Cut(tok, ":")
		if !found || value == "" {
			text = append(text, tok)
			continue
		}
		switch strings.ToLower(key) {
		case "assignee", "a":
			f.Assignee = value
		case "label", "l":
			f.Label = value
		case "priority", "p":
			f.Priority = value
		case "agent":
			f.Agent = value
		default:
			text = append(text, tok)
		}
	}
	f.Text = strings.Join(text, " ")
	return f
}

// Empty reports whether the filter has no predicates.
func (f Filter) Empty() bool { return f == Filter{} }

// Match reports whether t passes the filter. assignee is the display name of
// the task's assignee ("" when unassigned) and agent is the name of the agent
// currently working on it ("" when none).
func (f Filter) Match(t taskdomain.Task, assignee, agent string) bool {
	if f.Text != "" && !containsFold(t.Title, f.Text) && !containsFold(t.Description, f.Text) {
		return false
	}
	if f.Assignee != "" {
		id := ""
		if t.AssigneeID != nil {
			id = *t.AssigneeID
		}
		if !containsFold(assignee, f.Assignee) && !containsFold(id, f.Assignee) {
			return false
		}
	}
	if f.Label != "" && !hasLabel(t.Labels, f.Label) {
		return false
	}
	if f.Priority != "" && !strings.EqualFold(string(t.Priority), f.Priority) {
		return false
	}
	if f.Agent != "" && !containsFold(agent, f.Agent) {
		// An agent also "owns" the tasks assigned to it.
		if t.AssigneeID == nil || !containsFold(*t.AssigneeID, f.Agent) {
			return false
		}
	}
	return true
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, want) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return s != "" && strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
