package cli

import "testing"

// Cobra rejects undeclared flags before RunE, so --actor must be declared on
// the mcp command for connectors that pass an explicit agent identity.
func TestMCPCmdDeclaresActorAndReadOnlyFlags(t *testing.T) {
	cmd := newMCPCmd()
	if err := cmd.ParseFlags([]string{"--actor", "kiro-ana", "--read-only"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if got, _ := cmd.Flags().GetString("actor"); got != "kiro-ana" {
		t.Fatalf("actor = %q, want kiro-ana", got)
	}
}
