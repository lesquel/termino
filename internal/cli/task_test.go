package cli

import (
	"strings"
	"testing"

	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
)

func TestNormalizeTaskInput_StripsInjectionAndDefaultsPriority(t *testing.T) {
	title, desc, prio, err := normalizeTaskInput("\x1b]0;x\x07Fix\nlogin", "a\n\x1b[31mb\x07", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Fix login" || desc != "a\nb" || prio != taskdomain.PriorityMedium {
		t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", title, desc, prio, "Fix login", "a\nb", taskdomain.PriorityMedium)
	}
}

func TestNormalizeTaskInput_RejectsInvalidPriorityAndEmptyTitle(t *testing.T) {
	if _, _, _, err := normalizeTaskInput("Task", "", "banana"); err == nil || !strings.Contains(err.Error(), "priority") {
		t.Fatalf("err = %v, want invalid priority error", err)
	}
	if _, _, _, err := normalizeTaskInput("\x1b]0;x\x07", "", "high"); err == nil {
		t.Fatal("empty sanitized title accepted")
	}
}
