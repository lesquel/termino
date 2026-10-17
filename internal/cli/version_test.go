package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Release builds inject Version through -ldflags; `termino version` must print it.
func TestVersionCmdPrintsInjectedVersion(t *testing.T) {
	old := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = old })

	cmd := newVersionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("version: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "termino 1.2.3" {
		t.Fatalf("output = %q, want %q", got, "termino 1.2.3")
	}
}
