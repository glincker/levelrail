package deploysafety

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// reasonSources lists every file that declares status reason constants and
// the name prefix they share, so a new one cannot skip the catalog.
var reasonSources = []struct{ file, prefix string }{
	{"reasons.go", "Reason"},
	{"../store/deploy_attempt_safety.go", "DeployReason"},
	{"../store/deploy_attempt_queue.go", "DeployReason"},
}

func reasonConstants(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, src := range reasonSources {
		collectReasons(t, src.file, src.prefix, out)
	}
	return out
}

func collectReasons(t *testing.T, file, prefix string, out map[string]string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if !strings.HasPrefix(name.Name, prefix) || i >= len(vs.Values) {
				continue
			}
			lit, ok := vs.Values[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote %s: %v", name.Name, err)
			}
			out[name.Name] = v
		}
		return true
	})
}

func TestReasonCatalogCoversEveryReasonConstant(t *testing.T) {
	consts := reasonConstants(t)
	if len(consts) == 0 {
		t.Fatal("found no Reason constants in reasons.go")
	}
	seen := map[string]bool{}
	for name, code := range consts {
		info, ok := LookupReason(code)
		if !ok {
			t.Errorf("%s (%q) has no entry in reasonCatalog: add a plain explanation", name, code)
			continue
		}
		seen[code] = true
		if strings.TrimSpace(info.Meaning) == "" || strings.TrimSpace(info.NextStep) == "" || info.Kind == "" {
			t.Errorf("%s: meaning, next step and kind must all be set", code)
		}
	}
	for _, r := range reasonCatalog {
		if !seen[r.Code] {
			t.Errorf("catalog entry %q matches no Reason constant", r.Code)
		}
	}
	if len(reasonCatalog) != len(consts) {
		t.Errorf("catalog has %d entries, reasons.go has %d constants (duplicate code?)", len(reasonCatalog), len(consts))
	}
}

func TestStatusReasonsDocIsGenerated(t *testing.T) {
	got, err := os.ReadFile("../../docs/status-reasons.md")
	if err != nil {
		t.Fatalf("read docs/status-reasons.md: %v", err)
	}
	if string(got) != RenderReasonsMarkdown() {
		t.Fatal("docs/status-reasons.md is stale: run `go run ./scripts/gen-status-reasons`")
	}
}
