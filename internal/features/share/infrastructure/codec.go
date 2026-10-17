// Package infrastructure provides the TOML codec, filesystem store, git root
// lookup and SQLite hash store used to share a project through a repository.
package infrastructure

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/lesquel/termino/internal/features/share/domain"
)

// TOMLCodec encodes and decodes shared files. Encoding is hand written so the
// output is byte-for-byte deterministic: fixed key order, empty optional keys
// omitted, timestamps in UTC RFC 3339 and multi-line descriptions as """.
type TOMLCodec struct{}

// EncodeProject renders project.toml.
func (TOMLCodec) EncodeProject(p domain.ProjectFile) []byte {
	var w writer
	w.int("version", p.Version)
	w.str("key", p.Key)
	w.str("name", p.Name)
	w.str("methodology", p.Methodology)
	for _, c := range p.Columns {
		w.table("columns")
		w.str("name", c.Name)
		w.int("position", c.Position)
		w.int("wip_limit", c.WIPLimit)
	}
	for _, m := range p.Members {
		w.table("members")
		w.str("name", m.Name)
		w.optStr("kind", m.Kind)
	}
	for _, l := range p.Labels {
		w.table("labels")
		w.str("name", l.Name)
		w.optStr("color", l.Color)
	}
	for _, s := range p.Sprints {
		w.table("sprints")
		w.str("name", s.Name)
		w.optDate("start", s.Start)
		w.optDate("end", s.End)
		w.optStr("state", s.State)
	}
	return w.buf.Bytes()
}

// EncodeTask renders tasks/<id>.toml.
func (TOMLCodec) EncodeTask(t domain.TaskFile) []byte {
	var w writer
	w.int("version", t.Version)
	w.str("id", t.ID)
	w.int("number", t.Number)
	w.str("title", t.Title)
	w.str("column", t.Column)
	w.raw("position", strconv.FormatFloat(t.Position, 'f', -1, 64))
	w.str("priority", t.Priority)
	if t.Points != nil {
		w.int("points", *t.Points)
	}
	w.optStr("assignee", t.Assignee)
	if len(t.Labels) > 0 {
		quoted := make([]string, len(t.Labels))
		for i, l := range t.Labels {
			quoted[i] = basicString(l)
		}
		w.raw("labels", "["+strings.Join(quoted, ", ")+"]")
	}
	w.optStr("sprint", t.Sprint)
	if t.Due != nil {
		w.optDate("due", *t.Due)
	}
	w.raw("created_at", timestamp(t.CreatedAt))
	w.raw("updated_at", timestamp(t.UpdatedAt))
	w.optStr("updated_by", t.UpdatedBy)
	switch {
	case t.Description == "":
	case strings.Contains(t.Description, "\n"):
		// The newline after the opening quotes is trimmed by TOML and the one
		// before the closing quotes is trimmed by the decoder.
		w.raw("description", `"""`+"\n"+multilineBody(t.Description)+"\n"+`"""`)
	default:
		w.str("description", t.Description)
	}
	return w.buf.Bytes()
}

// writer accumulates TOML lines.
type writer struct{ buf bytes.Buffer }

func (w *writer) raw(key, value string) { fmt.Fprintf(&w.buf, "%s = %s\n", key, value) }
func (w *writer) str(key, value string) { w.raw(key, basicString(value)) }
func (w *writer) int(key string, v int) { w.raw(key, strconv.Itoa(v)) }

func (w *writer) optStr(key, value string) {
	if value != "" {
		w.str(key, value)
	}
}

func (w *writer) optDate(key string, t time.Time) {
	if !t.IsZero() {
		w.raw(key, t.UTC().Format(time.DateOnly))
	}
}

func (w *writer) table(name string) { fmt.Fprintf(&w.buf, "\n[[%s]]\n", name) }

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// basicString quotes s as a single-line TOML basic string.
func basicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			writeRune(&b, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// multilineBody escapes s for the inside of a multi-line basic string. A
// quote is escaped only when another quote follows it, which is enough to
// never produce the closing delimiter while keeping prose readable.
func multilineBody(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '"' && i+1 < len(s) && s[i+1] == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteByte('\n')
		default:
			writeRune(&b, r)
		}
	}
	return b.String()
}

// writeRune writes r, escaping control characters (tab and carriage return
// with their short forms) that TOML strings may not hold literally.
func writeRune(b *strings.Builder, r rune) {
	switch {
	case r == '\t':
		b.WriteString(`\t`)
	case r == '\r':
		b.WriteString(`\r`)
	case r < 0x20 || r == 0x7f:
		fmt.Fprintf(b, `\u%04X`, r)
	default:
		b.WriteRune(r)
	}
}

// --- decoding ---

type rawProject struct {
	Version     int    `toml:"version"`
	Key         string `toml:"key"`
	Name        string `toml:"name"`
	Methodology string `toml:"methodology"`
	Columns     []struct {
		Name     string `toml:"name"`
		Position int    `toml:"position"`
		WIPLimit int    `toml:"wip_limit"`
	} `toml:"columns"`
	Members []struct {
		Name string `toml:"name"`
		Kind string `toml:"kind"`
	} `toml:"members"`
	Labels []struct {
		Name  string `toml:"name"`
		Color string `toml:"color"`
	} `toml:"labels"`
	Sprints []struct {
		Name  string     `toml:"name"`
		Start *time.Time `toml:"start"`
		End   *time.Time `toml:"end"`
		State string     `toml:"state"`
	} `toml:"sprints"`
}

type rawTask struct {
	Version     int        `toml:"version"`
	ID          string     `toml:"id"`
	Number      int        `toml:"number"`
	Title       string     `toml:"title"`
	Column      string     `toml:"column"`
	Position    number     `toml:"position"`
	Priority    string     `toml:"priority"`
	Points      *int       `toml:"points"`
	Assignee    string     `toml:"assignee"`
	Labels      []string   `toml:"labels"`
	Sprint      string     `toml:"sprint"`
	Due         *time.Time `toml:"due"`
	CreatedAt   time.Time  `toml:"created_at"`
	UpdatedAt   time.Time  `toml:"updated_at"`
	UpdatedBy   string     `toml:"updated_by"`
	Description string     `toml:"description"`
}

// number accepts both TOML integers and floats (positions are fractional but
// are written without a fraction when whole).
type number float64

// UnmarshalTOML implements toml.Unmarshaler.
func (n *number) UnmarshalTOML(v any) error {
	switch x := v.(type) {
	case int64:
		*n = number(x)
	case float64:
		*n = number(x)
	default:
		return fmt.Errorf("expected a number, got %T", v)
	}
	return nil
}

// DecodeProject parses project.toml.
func (TOMLCodec) DecodeProject(data []byte) (domain.ProjectFile, error) {
	var raw rawProject
	if err := decode(data, &raw, &raw.Version); err != nil {
		return domain.ProjectFile{}, err
	}
	if strings.TrimSpace(raw.Key) == "" {
		return domain.ProjectFile{}, fmt.Errorf("%w: missing key", domain.ErrInvalidFile)
	}
	p := domain.ProjectFile{Version: raw.Version, Key: raw.Key, Name: raw.Name, Methodology: raw.Methodology}
	for _, c := range raw.Columns {
		p.Columns = append(p.Columns, domain.ColumnEntry{Name: c.Name, Position: c.Position, WIPLimit: c.WIPLimit})
	}
	for _, m := range raw.Members {
		p.Members = append(p.Members, domain.MemberEntry{Name: m.Name, Kind: m.Kind})
	}
	for _, l := range raw.Labels {
		p.Labels = append(p.Labels, domain.LabelEntry{Name: l.Name, Color: l.Color})
	}
	for _, s := range raw.Sprints {
		p.Sprints = append(p.Sprints, domain.SprintEntry{Name: s.Name, Start: dateOf(s.Start), End: dateOf(s.End), State: s.State})
	}
	return p, nil
}

// DecodeTask parses tasks/<id>.toml.
func (TOMLCodec) DecodeTask(data []byte) (domain.TaskFile, error) {
	var raw rawTask
	if err := decode(data, &raw, &raw.Version); err != nil {
		return domain.TaskFile{}, err
	}
	if strings.TrimSpace(raw.ID) == "" || strings.TrimSpace(raw.Title) == "" {
		return domain.TaskFile{}, fmt.Errorf("%w: missing id or title", domain.ErrInvalidFile)
	}
	t := domain.TaskFile{
		Version: raw.Version, ID: raw.ID, Number: raw.Number, Title: raw.Title, Column: raw.Column,
		Position: float64(raw.Position), Priority: raw.Priority, Points: raw.Points,
		Assignee: raw.Assignee, Labels: raw.Labels, Sprint: raw.Sprint,
		CreatedAt: raw.CreatedAt.UTC(), UpdatedAt: raw.UpdatedAt.UTC(), UpdatedBy: raw.UpdatedBy,
		Description: strings.TrimSuffix(raw.Description, "\n"),
	}
	if raw.Due != nil {
		d := dateOf(raw.Due)
		t.Due = &d
	}
	return t, nil
}

// decode rejects conflict markers, parses data into v and checks the version
// written at *version.
func decode(data []byte, v any, version *int) error {
	if hasConflictMarkers(data) {
		return domain.ErrConflictMarkers
	}
	if _, err := toml.Decode(string(data), v); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidFile, err)
	}
	switch {
	case *version > domain.FormatVersion:
		return fmt.Errorf("%w: %d (this termino reads up to %d)", domain.ErrUnsupportedVersion, *version, domain.FormatVersion)
	case *version < 1:
		return fmt.Errorf("%w: missing version", domain.ErrInvalidFile)
	}
	return nil
}

// hasConflictMarkers reports whether a line starts with a git conflict
// marker. "=======" alone is not checked: it is a valid Markdown underline.
func hasConflictMarkers(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("<<<<<<<")) || bytes.HasPrefix(line, []byte(">>>>>>>")) {
			return true
		}
	}
	return false
}

// dateOf returns the calendar date of t as UTC midnight; nil gives zero.
// TOML local dates carry no zone, so the wall-clock fields are used as is.
func dateOf(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
