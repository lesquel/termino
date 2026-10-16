package sanitize

import (
	"strings"
	"testing"
)

func TestStripRemovesCSISequences(t *testing.T) {
	in := "\x1b[31mred\x1b[0m text"
	want := "red text"
	if got := Strip(in); got != want {
		t.Errorf("Strip() = %q, want %q", got, want)
	}
}

func TestStripRemovesCursorAndScreenSequences(t *testing.T) {
	in := "a\x1b[2K\x1b[H\x1b[2Jb"
	want := "ab"
	if got := Strip(in); got != want {
		t.Errorf("Strip() = %q, want %q", got, want)
	}
}

func TestStripRemovesOSCTitleSequence(t *testing.T) {
	in := "\x1b]0;evil title\x07body"
	want := "body"
	if got := Strip(in); got != want {
		t.Errorf("Strip() = %q, want %q", got, want)
	}
}

func TestStripRemovesOSCBellessTerminated(t *testing.T) {
	in := "\x1b]0;title\x1b\\body"
	want := "body"
	if got := Strip(in); got != want {
		t.Errorf("Strip() = %q, want %q", got, want)
	}
}

func TestCleanDropsControlCharacters(t *testing.T) {
	in := "a\x00b\x07c\x01d"
	want := "abcd"
	if got := Clean(in); got != want {
		t.Errorf("Clean() = %q, want %q", got, want)
	}
}

func TestCleanKeepsNewlinesAndTabs(t *testing.T) {
	in := "line1\nline2\tindent"
	if got := Clean(in); got != in {
		t.Errorf("Clean() = %q, want %q", got, in)
	}
}

func TestCleanDropsStrayEscapeAndC1(t *testing.T) {
	in := "a\x1bb\x85c" // \x1b stray ESC, \x85 NEL (C1)
	want := "abc"
	if got := Clean(in); got != want {
		t.Errorf("Clean() = %q, want %q", got, want)
	}
}

func TestLineCollapsesToSingleLine(t *testing.T) {
	in := "\x1b[32mhello\nworld\x1b[0m"
	got := Line(in)
	if strings.Contains(got, "\n") {
		t.Errorf("Line() still contains newline: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("Line() dropped printable text: %q", got)
	}
}

func TestLineTrimsWhitespace(t *testing.T) {
	in := "  \x1b[0m  title  \n"
	want := "title"
	if got := Line(in); got != want {
		t.Errorf("Line() = %q, want %q", got, want)
	}
}

func TestLineDropsTerminalTitleInjection(t *testing.T) {
	// An agent trying to reprogram the terminal title must produce nothing.
	in := "\x1b]0;pwned\x07"
	if got := Line(in); got != "" {
		t.Errorf("Line() = %q, want empty", got)
	}
}
