package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// goStringConsts returns the string values of every const in path whose name
// starts with prefix.
func goStringConsts(t *testing.T, path, prefix string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, prefix) || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if v, err := strconv.Unquote(lit.Value); err == nil {
					out = append(out, v)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// tsUnionMembers returns the string literals of `export type name = 'a' | 'b'`.
func tsUnionMembers(t *testing.T, path, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixed repo-relative path in a test
	if err != nil {
		t.Skipf("frontend source not available: %v", err)
	}
	decl := regexp.MustCompile(`(?s)export type ` + regexp.QuoteMeta(name) + `\s*=(.*?)\n\n|export type ` + regexp.QuoteMeta(name) + `\s*=([^\n]*)\n`)
	m := decl.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("type %s not found in %s", name, path)
	}
	body := m[1] + m[2]
	var out []string
	for _, lit := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(body, -1) {
		out = append(out, lit[1])
	}
	slices.Sort(out)
	return out
}

// TestWireEnumsMatchFrontend fails when a value the backend can emit has no
// matching member in the web app's union, which is how a label map ends up
// rendering "undefined". Adding a Go constant now forces the same edit in TS.
func TestWireEnumsMatchFrontend(t *testing.T) {
	cases := []struct{ goFile, goPrefix, tsFile, tsType string }{
		{"../store/deploy_attempt.go", "DeployAttemptSource", "../../web/src/types/deployAttempt.ts", "DeployAttemptSource"},
		{"../store/deploy_attempt.go", "DeployAttemptStatus", "../../web/src/types/deployAttempt.ts", "DeployAttemptStatus"},
	}
	for _, tc := range cases {
		t.Run(tc.tsType, func(t *testing.T) {
			goVals := goStringConsts(t, tc.goFile, tc.goPrefix)
			tsVals := tsUnionMembers(t, tc.tsFile, tc.tsType)
			if len(goVals) == 0 {
				t.Fatalf("no Go constants found with prefix %s", tc.goPrefix)
			}
			if !slices.Equal(goVals, tsVals) {
				t.Errorf("backend emits %v but the frontend type %s lists %v", goVals, tc.tsType, tsVals)
			}
		})
	}
}
