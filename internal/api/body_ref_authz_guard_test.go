package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// resourceRefTags are JSON fields and query keys that name another app,
// database, project, environment or node.
var resourceRefTags = map[string]bool{
	"app": true, "app_name": true, "apps": true, "source_app": true, "target_app": true,
	"database": true, "database_name": true, "databases": true, "backup_database_name": true,
	"project_id": true, "environment_id": true,
	"node_id": true, "target_node_id": true,
}

// bodyRefAuthzAllowlist names handlers that read a resource-shaped field but
// need no per-reference check. Every entry carries its reason.
var bodyRefAuthzAllowlist = map[string]string{ //nolint:gosec // reasons, not credentials
	"handleUpdateApp":           "node_id, project_id, environment_id and database_env are response-only on update; existing values are kept",
	"handlePreflightApp":        "only required_env is read from the body; placement comes from the stored app",
	"handleCreateToken":         "resolveTokenScope only narrows to a team and project the minting user already belongs to",
	"handleIaCExport":           "runs through loopbackDoer, so every read re-enters the API with the caller's credentials",
	"handleStepUp":              "a step-up grant confers no access; the protected route still authorizes the app",
	"handleStepUpPasskeyFinish": "a step-up grant confers no access; the protected route still authorizes the app",
	"handleCIOIDCExchange":      "unauthenticated by design; the app name only selects trusts, and a token is minted only after the CI provider JWT verifies against a trust of that app",
	"parseDeploymentQuery":      "pure parser; handleListDeployments intersects the filter with visibleAppNames",
}

func TestBodyResourceRefsAreAuthorized(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var parsed []*ast.File
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // package-local source files
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, af)
	}
	shaped := resourceShapedTypes(parsed)
	decoders := map[string]bool{}
	funcs := map[string]*ast.FuncDecl{}
	for _, af := range parsed {
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				funcs[funcKey(fd)] = fd
			}
		}
	}
	authorizing := authorizingFuncs(funcs)
	for _, fd := range funcs {
		if decodesShaped(fd, shaped, nil) && returnsShaped(fd, shaped) && !authorizing[fd.Name.Name] {
			decoders[fd.Name.Name] = true
		}
	}
	var missing []string
	seen := map[string]bool{}
	for k, fd := range funcs {
		if decoders[fd.Name.Name] {
			continue
		}
		if !decodesShaped(fd, shaped, decoders) && !readsRefQuery(fd) {
			continue
		}
		seen[k] = true
		if callsAny(fd, authorizing) {
			continue
		}
		if _, ok := bodyRefAuthzAllowlist[k]; ok {
			continue
		}
		missing = append(missing, k)
	}
	sort.Strings(missing)
	for _, k := range missing {
		t.Errorf("%s reads a resource reference from the body or query without an authorize helper; call rt.authorizeRefs or allowlist it with a reason", k)
	}
	for k := range bodyRefAuthzAllowlist {
		if !seen[k] {
			t.Errorf("allowlist entry %s no longer reads a resource reference; remove it", k)
		}
	}
}

func funcKey(fd *ast.FuncDecl) string {
	return fd.Name.Name
}

func resourceShapedTypes(files []*ast.File) map[string]bool {
	structs := map[string]*ast.StructType{}
	for _, af := range files {
		ast.Inspect(af, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					structs[ts.Name.Name] = st
				}
			}
			return true
		})
	}
	shaped := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for name, st := range structs {
			if shaped[name] {
				continue
			}
			for _, f := range st.Fields.List {
				if fieldIsRef(f) || shaped[baseTypeName(f.Type)] {
					shaped[name] = true
					changed = true
					break
				}
			}
		}
	}
	return shaped
}

func fieldIsRef(f *ast.Field) bool {
	if f.Tag == nil {
		return false
	}
	raw, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		return false
	}
	name, _, _ := strings.Cut(reflect.StructTag(raw).Get("json"), ",")
	return resourceRefTags[name]
}

func baseTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return baseTypeName(v.X)
	case *ast.ArrayType:
		return baseTypeName(v.Elt)
	case *ast.MapType:
		return baseTypeName(v.Value)
	}
	return ""
}

// decodesShaped reports whether fd decodes a request into a resource-shaped
// type, directly or through a decoder helper that returns one.
func decodesShaped(fd *ast.FuncDecl, shaped, decoders map[string]bool) bool {
	vars := map[string]bool{}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ValueSpec:
			if shaped[baseTypeName(v.Type)] {
				for _, id := range v.Names {
					vars[id.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range v.Rhs {
				if i < len(v.Lhs) && shaped[compositeTypeName(rhs)] {
					if id, ok := v.Lhs[i].(*ast.Ident); ok {
						vars[id.Name] = true
					}
				}
			}
		}
		return true
	})
	for _, fl := range []*ast.FieldList{fd.Type.Params, fd.Type.Results} {
		if fl == nil {
			continue
		}
		for _, p := range fl.List {
			if shaped[baseTypeName(p.Type)] {
				for _, id := range p.Names {
					vars[id.Name] = true
				}
			}
		}
	}
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		name := callName(call)
		if decoders[name] {
			found = true
			return false
		}
		if !strings.Contains(strings.ToLower(name), "decode") && name != "Unmarshal" {
			return true
		}
		for _, a := range call.Args {
			if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
				if id, ok := u.X.(*ast.Ident); ok && vars[id.Name] {
					found = true
				}
			}
			if id, ok := a.(*ast.Ident); ok && vars[id.Name] {
				found = true
			}
		}
		return true
	})
	return found
}

func compositeTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return baseTypeName(v.Type)
	case *ast.UnaryExpr:
		return compositeTypeName(v.X)
	}
	return ""
}

func returnsShaped(fd *ast.FuncDecl, shaped map[string]bool) bool {
	if fd.Type.Results == nil {
		return false
	}
	for _, r := range fd.Type.Results.List {
		if shaped[baseTypeName(r.Type)] {
			return true
		}
	}
	return false
}

func readsRefQuery(fd *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || callName(call) != "Get" || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if hs, ok := sel.X.(*ast.SelectorExpr); ok && hs.Sel.Name == "Header" {
			return true
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok {
			if s, err := strconv.Unquote(lit.Value); err == nil && resourceRefTags[s] {
				found = true
			}
		}
		return true
	})
	return found
}

// authorizingFuncs is every package function that reaches an authorize*
// decision within authorizeDepth calls, so a handler may delegate the check
// to a validator such as validateProjectID.
func authorizingFuncs(funcs map[string]*ast.FuncDecl) map[string]bool {
	const authorizeDepth = 3
	out := map[string]bool{}
	for name := range funcs {
		if strings.HasPrefix(name, "authorize") {
			out[name] = true
		}
	}
	for range authorizeDepth {
		next := map[string]bool{}
		for name, fd := range funcs {
			if out[name] || callsAny(fd, out) {
				next[name] = true
			}
		}
		out = next
	}
	return out
}

func callsAny(fd *ast.FuncDecl, names map[string]bool) bool {
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			name := callName(call)
			if names[name] || strings.HasPrefix(name, "authorize") {
				found = true
			}
		}
		return !found
	})
	return found
}

func callName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.IndexExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}
