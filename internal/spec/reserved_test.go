package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var namespaceNameRe = regexp.MustCompile(`(?i)(SecretsKey|SecretsNS|SecretsService|SecretService|SecretsOwner|SecretNS|KeyService|SecretPrefix|SecretsNamespace|SaltService)`)

// managerCalls are the secret Manager methods whose second argument is the namespace; classifyManagerCalls covers every other method.
var managerCalls = map[string]bool{"SetValue": true, "SetValueGuarded": true, "Resolve": true, "Exists": true, "DeleteValue": true, "SetLocked": true}

func stringLit(e ast.Expr) (string, bool) {
	if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
		return stringLit(b.X)
	}
	l, ok := e.(*ast.BasicLit)
	if !ok || l.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(l.Value)
	return s, err == nil
}

// A namespace literal passed to the secret manager must be covered by the reserved list, so a new one cannot be forgotten.
func TestReservedNamesCoverSecretNamespaces(t *testing.T) {
	fset := token.NewFileSet()
	checked := 0
	literalCalls := 0
	check := func(pos token.Pos, lit string) {
		if len(lit) < 2 || !nameLike.MatchString(lit) {
			return
		}
		checked++
		if !ReservedNameCovers(lit) {
			t.Errorf("%s: secret namespace %q is not in the reserved list (internal/spec/reserved.go)", fset.Position(pos), lit)
		}
	}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 3 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !managerCalls[sel.Sel.Name] {
				return true
			}
			if s, ok := stringLit(call.Args[1]); ok {
				literalCalls++
				if !ReservedNameCovers(s) {
					t.Errorf("%s: %s uses namespace literal %q that is not reserved", fset.Position(call.Pos()), sel.Sel.Name, s)
				}
			}
			return true
		})
		for _, decl := range f.Decls {
			switch dd := decl.(type) {
			case *ast.FuncDecl:
				if dd.Body == nil || !namespaceNameRe.MatchString(dd.Name.Name) {
					continue
				}
				ast.Inspect(dd.Body, func(n ast.Node) bool {
					if r, ok := n.(*ast.ReturnStmt); ok && len(r.Results) == 1 {
						if s, ok := stringLit(r.Results[0]); ok {
							check(r.Pos(), s)
						}
					}
					return true
				})
			case *ast.GenDecl:
				for _, sp := range dd.Specs {
					vs, ok := sp.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range vs.Names {
						if i < len(vs.Values) && namespaceNameRe.MatchString(name.Name) {
							if s, ok := stringLit(vs.Values[i]); ok {
								check(name.Pos(), s)
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 10 {
		t.Fatalf("only %d namespace literals found, the scan is broken", checked)
	}
	classifyManagerCalls(t)
}

func TestIsReservedName(t *testing.T) {
	for _, n := range []string{"vault", "email-settings", "mail-system-email", "ai-assistant", "cloudflare-dns", "dns-hetzner", "builtin-registry", "route53-dns", "ai-assistant-provider-x", "heartbeat", "traffic-export", "dns-server", "monitor-web"} {
		if !IsReservedName(n) {
			t.Errorf("%q should be reserved", n)
		}
		if ValidateServiceName(n) == nil {
			t.Errorf("ValidateServiceName(%q) should refuse", n)
		}
	}
	for _, n := range []string{"web", "email", "dnsmasq", "vault-ui", "monitor"} {
		if IsReservedName(n) {
			t.Errorf("%q should be free", n)
		}
	}
}
