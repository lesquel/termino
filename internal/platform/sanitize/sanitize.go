// Package sanitize strips ANSI escape sequences and control characters from
// untrusted text before it is stored or rendered. Termino accepts task text
// from AI agents over MCP; without sanitization an agent could inject escape
// sequences that manipulate the TUI (TRD security 11.3-11.4).
//
// Sanitize at the input edge (MCP/CLI handlers, before use cases) and at the
// output edge (before rendering task text in the TUI).
package sanitize

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ansiRe matches the ANSI escape sequences that matter for terminal safety:
//   - OSC (Operating System Command) sequences such as "\x1b]0;title\x07",
//     which can reprogram the terminal title or clipboard.
//   - CSI (Control Sequence Introducer) sequences such as "\x1b[31m", which
//     carry SGR colours but also cursor movement and screen clearing.
//   - A couple of single-character ESC introducers.
var ansiRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[=>]`)

// Strip removes ANSI escape sequences from s, leaving all other bytes intact.
func Strip(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// Clean removes ANSI escape sequences and control characters from s, keeping
// the printable whitespace runs \t, \n, and \r. C0 control characters
// (0x00-0x1f except tab/newline/return), DEL (0x7f), and C1 control characters
// (0x80-0x9f) are dropped. A stray ESC not part of a recognised sequence is
// also dropped here (it is a control character).
func Clean(s string) string {
	s = Strip(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		// Invalid UTF-8 bytes (e.g. a raw C1 byte such as 0x85) decode as
		// RuneError with width 1; drop them instead of rendering U+FFFD.
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
				continue
			}
		}
		switch r {
		case '\t', '\n', '\r':
			b.WriteRune(r)
			continue
		}
		// Drop C0 controls (except the whitespace handled above), DEL, and C1.
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Line returns a single-line, sanitized string suitable for one-line displays
// such as a board card. It applies Clean and then collapses newlines, carriage
// returns, and tabs to spaces, trimming surrounding whitespace.
func Line(s string) string {
	s = Clean(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}
