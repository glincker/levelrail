package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const helpGapsFile = "testdata/help_flag_gaps.txt"

var flagNameArg = map[string]int{
	"String": 0, "Int": 0, "Bool": 0, "Duration": 0, "Float64": 0, "Int64": 0, "Uint": 0, "BoolFunc": 0, "Func": 0,
	"StringVar": 1, "IntVar": 1, "BoolVar": 1, "DurationVar": 1, "Float64Var": 1, "Int64Var": 1, "UintVar": 1, "Var": 1,
}

func stringLits(n ast.Node) []string {
	var out []string
	ast.Inspect(n, func(x ast.Node) bool {
		if bl, ok := x.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if s, err := strconv.Unquote(bl.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

func isFlagSetCtor(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		id, ok := fn.X.(*ast.Ident)
		return ok && id.Name == "flag" && fn.Sel.Name == "NewFlagSet"
	case *ast.Ident:
		return fn.Name == "apiFlagSet"
	}
	return false
}

// helpFlagGaps finds flags a verb defines on its FlagSet that its own usage text never mentions.
func helpFlagGaps(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	decls := map[string]*ast.FuncDecl{}
	type unit struct {
		file string
		fn   *ast.FuncDecl
	}
	var units []unit
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil && fd.Recv == nil {
				decls[fd.Name.Name] = fd
				units = append(units, unit{f, fd})
			}
		}
	}

	var gaps []string
	for _, u := range units {
		var fsName string
		ast.Inspect(u.fn.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Rhs) != 1 || len(as.Lhs) == 0 {
				return true
			}
			if call, ok := as.Rhs[0].(*ast.CallExpr); ok && isFlagSetCtor(call) {
				if id, ok := as.Lhs[0].(*ast.Ident); ok && fsName == "" {
					fsName = id.Name
				}
			}
			return true
		})
		if fsName == "" {
			continue
		}
		flags := map[string]bool{}
		var usageText []string
		hasUsage := false
		ast.Inspect(u.fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !ok || id.Name != fsName {
					return true
				}
				idx, ok := flagNameArg[sel.Sel.Name]
				if !ok || len(x.Args) <= idx {
					return true
				}
				if bl, ok := x.Args[idx].(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if s, err := strconv.Unquote(bl.Value); err == nil {
						flags[s] = true
					}
				}
			case *ast.AssignStmt:
				if len(x.Lhs) != 1 || len(x.Rhs) != 1 {
					return true
				}
				sel, ok := x.Lhs[0].(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Usage" {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !ok || id.Name != fsName {
					return true
				}
				hasUsage = true
				usageText = append(usageText, stringLits(x.Rhs[0])...)
				ast.Inspect(x.Rhs[0], func(m ast.Node) bool {
					if c, ok := m.(*ast.CallExpr); ok {
						if id, ok := c.Fun.(*ast.Ident); ok {
							if d := decls[id.Name]; d != nil {
								usageText = append(usageText, stringLits(d.Body)...)
							}
						}
					}
					return true
				})
			}
			return true
		})
		if !hasUsage || len(flags) == 0 {
			continue
		}
		text := strings.Join(usageText, "\n")
		for name := range flags {
			if len(name) == 1 {
				continue
			}
			re := regexp.MustCompile(`(^|[^-\w])--` + regexp.QuoteMeta(name) + `([^\w-]|$)`)
			if !re.MatchString(text) {
				gaps = append(gaps, u.file+" "+u.fn.Name.Name+" --"+name)
			}
		}
	}
	sort.Strings(gaps)
	return gaps
}

// TestHelpTextCoversEveryFlag fails when a verb defines a flag its -h text omits.
// Existing omissions are frozen in testdata/help_flag_gaps.txt: fix one, then remove its line.
func TestHelpTextCoversEveryFlag(t *testing.T) {
	gaps := helpFlagGaps(t)
	if os.Getenv("UPDATE_HELP_FLAG_GAPS") != "" {
		if err := os.WriteFile(helpGapsFile, []byte(strings.Join(gaps, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(helpGapsFile)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l != "" {
			known[l] = true
		}
	}
	now := map[string]bool{}
	for _, g := range gaps {
		now[g] = true
		if !known[g] {
			t.Errorf("flag missing from -h text: %s (add it to the verb's usage)", g)
		}
	}
	for k := range known {
		if !now[k] {
			t.Errorf("stale entry in %s, the flag is now documented: %s", helpGapsFile, k)
		}
	}
}
