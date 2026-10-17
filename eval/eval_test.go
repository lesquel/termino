// Package eval checks the PRD 6 agent evaluation set in prompts.md against
// the MCP server: the set must stay runnable as the tool surface evolves.
package eval

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	appmcp "github.com/lesquel/termino/internal/app/mcp"
)

const wantCases = 20

type evalCase struct {
	id     string
	fields map[string]string
}

var (
	caseHeading = regexp.MustCompile(`^### (E\d{2}) `)
	fieldLine   = regexp.MustCompile(`^- \*\*(Prompt|Calls|Expected|Pass):\*\* (.+)$`)
	toolCall    = regexp.MustCompile("`([a-z_]+)\\(([^)]*)\\)`")
)

func parseCases(t *testing.T) []evalCase {
	t.Helper()
	raw, err := os.ReadFile("prompts.md")
	if err != nil {
		t.Fatalf("read prompts.md: %v", err)
	}
	var cases []evalCase
	for _, line := range strings.Split(string(raw), "\n") {
		if m := caseHeading.FindStringSubmatch(line); m != nil {
			cases = append(cases, evalCase{id: m[1], fields: map[string]string{}})
			continue
		}
		if m := fieldLine.FindStringSubmatch(line); m != nil && len(cases) > 0 {
			cases[len(cases)-1].fields[m[1]] = m[2]
		}
	}
	return cases
}

// toolSchemas lists the tools of a full (read-write) server through an
// in-process MCP client, the same way an agent discovers them.
func toolSchemas(t *testing.T) map[string]map[string]any {
	t.Helper()
	c, err := client.NewInProcessClient(appmcp.NewServer(appmcp.Deps{}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	init := mcplib.InitializeRequest{}
	init.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcplib.Implementation{Name: "eval", Version: "1.0"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	res, err := c.ListTools(ctx, mcplib.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	out := make(map[string]map[string]any, len(res.Tools))
	for _, tool := range res.Tools {
		out[tool.Name] = tool.InputSchema.Properties
	}
	return out
}

func TestEvalSetHasTwentyCompleteCases(t *testing.T) {
	cases := parseCases(t)
	if len(cases) != wantCases {
		t.Fatalf("prompts.md has %d cases, want %d (PRD 6)", len(cases), wantCases)
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.id] {
			t.Errorf("duplicate case id %s", c.id)
		}
		seen[c.id] = true
		for _, f := range []string{"Prompt", "Calls", "Expected", "Pass"} {
			if strings.TrimSpace(c.fields[f]) == "" {
				t.Errorf("%s: missing %s", c.id, f)
			}
		}
	}
}

func TestEvalSetReferencesExistingToolsAndArguments(t *testing.T) {
	tools := toolSchemas(t)
	used := map[string]bool{}
	for _, c := range parseCases(t) {
		calls := toolCall.FindAllStringSubmatch(c.fields["Calls"], -1)
		if len(calls) == 0 {
			t.Errorf("%s: no tool calls in %q", c.id, c.fields["Calls"])
		}
		for _, call := range calls {
			name, args := call[1], call[2]
			props, ok := tools[name]
			if !ok {
				t.Errorf("%s: unknown MCP tool %q", c.id, name)
				continue
			}
			used[name] = true
			for _, arg := range strings.Split(args, ",") {
				arg = strings.TrimSpace(arg)
				if arg == "" {
					continue
				}
				if _, ok := props[arg]; !ok {
					t.Errorf("%s: tool %s has no argument %q", c.id, name, arg)
				}
			}
		}
	}
	// The set should exercise the whole tool surface an agent sees.
	for name := range tools {
		if !used[name] {
			t.Errorf("no eval case uses MCP tool %q", name)
		}
	}
}
