package tui

import (
	"strings"
	"testing"
)

func TestShareStepOnlyInsideGitRepo(t *testing.T) {
	st := firstRunState()
	st.ShareRoot = "/work"
	r := &recorder{}
	m := newModel(t, st, r)
	m = press(m, "enter", "enter", "enter", "enter", "enter", "enter", "enter", "enter") // welcome … widget
	v := view(m)
	if !strings.Contains(v, "Share this board with the repo?") || !strings.Contains(v, "Step 8 of 9") || !strings.Contains(v, "/work/.termino") {
		t.Fatalf("share step:\n%s", v)
	}
	if m.Input().Share {
		t.Error("share must default to no")
	}
	m = press(m, "y", "enter")
	v = view(m)
	if !strings.Contains(v, "Step 9 of 9") || !strings.Contains(v, "Share") || !strings.Contains(v, "/work/.termino") {
		t.Fatalf("summary:\n%s", v)
	}
	m = press(m, "esc")
	if !strings.Contains(view(m), "Share this board") {
		t.Fatalf("back from summary should return to the share step:\n%s", view(m))
	}
	m = press(m, "enter", "enter")
	if r.got == nil || !r.got.Share {
		t.Fatalf("applied %+v, want Share", r.got)
	}
}

func TestNoShareStepOutsideGitRepo(t *testing.T) {
	r := &recorder{}
	m := newModel(t, firstRunState(), r)
	m = press(m, "enter", "enter", "enter", "enter", "enter", "enter", "enter", "enter")
	if v := view(m); !strings.Contains(v, "Review your setup") || strings.Contains(v, "Share") {
		t.Fatalf("expected the summary without sharing:\n%s", v)
	}
	m = press(m, "esc")
	if !strings.Contains(view(m), "shell widget") {
		t.Fatalf("back from summary should return to the widget step:\n%s", view(m))
	}
}
