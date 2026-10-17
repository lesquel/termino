// Package tuikittest holds test helpers shared by the TUI packages.
package tuikittest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/lesquel/termino/internal/tuikit"
)

// CaseKeys returns the string literals of every case clause inside the
// named functions (or methods) of the Go file at path: the keys those
// handlers switch on.
func CaseKeys(t testing.TB, path string, funcs ...string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	want := map[string]bool{}
	for _, name := range funcs {
		want[name] = true
	}
	var keys []string
	found := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !want[fn.Name.Name] {
			continue
		}
		found[fn.Name.Name] = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if s, err := strconv.Unquote(lit.Value); err == nil {
					keys = append(keys, s)
				}
			}
			return true
		})
	}
	for _, name := range funcs {
		if !found[name] {
			t.Fatalf("%s: function %s not found", path, name)
		}
	}
	return keys
}

// AssertDocumented fails for every key that none of the named help
// sections covers.
func AssertDocumented(t testing.TB, keys []string, sections ...string) {
	t.Helper()
	if len(keys) == 0 {
		t.Fatal("no handled keys found; the handler layout changed")
	}
	for _, k := range keys {
		covered := false
		for _, title := range sections {
			s, ok := tuikit.HelpSectionNamed(title)
			if !ok {
				t.Fatalf("no help section %q", title)
			}
			if s.Covers(k) {
				covered = true
			}
		}
		if !covered {
			t.Errorf("key %q is handled but missing from help sections %v", k, sections)
		}
	}
}
