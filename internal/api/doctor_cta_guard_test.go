package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

// doctorNoCTAAllowlist freezes the warn-capable doctor code paths that still
// give information but no next step.
// shrink only: remove an entry when the check gets a CTA.
var doctorNoCTAAllowlist = []string{
	"doctor.go:doctorCheckDataDirWritable", // no docs path or fix command: the remedy depends on how the host was set up
	"doctor.go:doctorCheckDatabase",        // a failed database ping has no safe generic remedy and no docs page yet
	"doctor.go:doctorCheckDocker",          // an unreachable engine has host-specific causes and no docs link yet
	"doctor.go:doctorCheckPort",            // the wrapper doctorCheckPortCtx names the holder, this base row has no link
	"doctor_checks_extra.go:doctorFromFix", // the row is built from a fix preview, its CTA is that fix but it is attached by a different id
}

// doctorOrphanFixes freezes typed fixes whose check id matches no code a doctor row can carry.
// shrink only: remove an entry when the fix id and the row code line up.
var doctorOrphanFixes = []string{
	"database_connections_high", // an alert kind, not a doctor row, so the fix never shows on Status
}

type doctorCTAUnit struct {
	key      string
	warn     bool
	hasCTA   bool
	typedFix bool
	codes    []string
}

// doctorCTASources parses the non-test sources of the api package.
func doctorCTASources(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = f
	}
	return files
}

func doctorNetworkSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	entries, err := os.ReadDir("../network")
	if err != nil {
		t.Fatalf("read network dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, "../network/"+name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = f
	}
	return files
}

// doctorStringConsts collects only package-level declarations: nested consts such as a
// function-local "code" would otherwise leak across functions in map-iteration order.
func doctorStringConsts(files map[string]*ast.File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						break
					}
					if s, ok := doctorResolveString(vs.Values[i], nil); ok {
						out[name.Name] = s
					}
				}
			}
		}
	}
	return out
}

func doctorResolveString(e ast.Expr, consts map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			s, err := strconv.Unquote(v.Value)
			return s, err == nil
		}
	case *ast.Ident:
		s, ok := consts[v.Name]
		return s, ok
	}
	return "", false
}

func doctorLocalSpecs(body *ast.BlockStmt) []ast.Spec {
	var specs []ast.Spec
	ast.Inspect(body, func(n ast.Node) bool {
		if vs, ok := n.(*ast.ValueSpec); ok {
			specs = append(specs, vs)
		}
		return true
	})
	return specs
}

func doctorIsEmptyString(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``")
}

func doctorSelectorName(e ast.Expr) string {
	if s, ok := e.(*ast.SelectorExpr); ok {
		return s.Sel.Name
	}
	return ""
}

// doctorCTAUnits reports, per function that builds doctor rows, whether it can
// warn or fail and whether it names a next step on the row.
func doctorCTAUnits(files map[string]*ast.File, consts map[string]string, serves func(string) bool) []doctorCTAUnit {
	var out []doctorCTAUnit
	for file, f := range files {
		if file == "demo_stubs.go" {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			u := doctorCTAUnit{key: file + ":" + fn.Name.Name}
			// Most checks declare their code as a function-local const.
			local := map[string]string{}
			for k, v := range consts {
				local[k] = v
			}
			for k, v := range doctorStringConsts(map[string]*ast.File{"fn": {Decls: []ast.Decl{&ast.GenDecl{Tok: token.CONST, Specs: doctorLocalSpecs(fn.Body)}}}}) {
				local[k] = v
			}
			buildsRow := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.CompositeLit:
					if id, ok := v.Type.(*ast.Ident); ok && id.Name == "doctorCheckResource" {
						buildsRow = true
					}
				case *ast.Ident:
					if v.Name == "doctorStatusWarn" || v.Name == "doctorStatusFail" {
						u.warn = true
					}
				case *ast.KeyValueExpr:
					key, _ := v.Key.(*ast.Ident)
					if key == nil {
						break
					}
					switch key.Name {
					case "DocsPath", "Fix":
						if !doctorIsEmptyString(v.Value) {
							u.hasCTA = true
						}
					case "Code":
						if s, ok := doctorResolveString(v.Value, local); ok {
							u.codes = append(u.codes, s)
						}
					}
				case *ast.AssignStmt:
					for i, lhs := range v.Lhs {
						name := doctorSelectorName(lhs)
						if (name == "DocsPath" || name == "Fix") && i < len(v.Rhs) && !doctorIsEmptyString(v.Rhs[i]) {
							u.hasCTA = true
						}
					}
				}
				return true
			})
			if !buildsRow || !u.warn {
				continue
			}
			// Same predicate as attachDoctorFixes: a fix reaches a row when it serves the row Code (id, alias or prefix).
			for _, c := range u.codes {
				if serves != nil && serves(c) {
					u.typedFix = true
				}
			}
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// doctorFixServes returns the fixes and the engine's own matching (Fix.ServesCode), so the guard
// follows the same alias rules attachDoctorFixes uses.
func doctorFixServes(t *testing.T) (func(string) bool, []fixit.Fix) {
	t.Helper()
	rt, _ := newDoctorTestRouter(t)
	fixes := rt.fixitFixes()
	return func(code string) bool {
		for _, f := range fixes {
			if f.ServesCode(code) {
				return true
			}
		}
		return false
	}, fixes
}

func TestDoctorCTAGuard_EveryWarnHasNextStep(t *testing.T) {
	files := doctorCTASources(t)
	serves, _ := doctorFixServes(t)
	units := doctorCTAUnits(files, doctorStringConsts(files), serves)
	allowed := map[string]bool{}
	for _, k := range doctorNoCTAAllowlist {
		allowed[k] = true
	}
	var offenders []string
	byKey := map[string]doctorCTAUnit{}
	for _, u := range units {
		byKey[u.key] = u
		if !u.hasCTA && !u.typedFix {
			offenders = append(offenders, u.key)
			if !allowed[u.key] {
				t.Errorf("%s can warn or fail with no typed fix, docs path or fix command; add one or list it in doctorNoCTAAllowlist with a reason", u.key)
			}
		}
	}
	for _, k := range doctorNoCTAAllowlist {
		u, ok := byKey[k]
		switch {
		case !ok:
			t.Errorf("allowlist entry %s no longer builds a warn-capable row, remove it", k)
		case u.hasCTA || u.typedFix:
			t.Errorf("allowlist entry %s now has a call to action, remove it", k)
		}
	}
	if !sort.StringsAreSorted(doctorNoCTAAllowlist) {
		t.Error("doctorNoCTAAllowlist must stay sorted")
	}
	if t.Failed() {
		t.Logf("current offenders (%d):\n\t%q", len(offenders), strings.Join(offenders, "\",\n\t\""))
	}
}

func TestDoctorCTAGuard_FixesReachARow(t *testing.T) {
	files := doctorCTASources(t)
	consts := doctorStringConsts(files)
	codes := map[string]bool{}
	for _, u := range doctorCTAUnits(files, consts, nil) {
		for _, c := range u.codes {
			codes[c] = true
		}
	}
	// Mesh rows take their code from the network readiness ids, so those constants count as row codes.
	netFiles := doctorNetworkSources(t)
	literals := map[string]bool{}
	for _, f := range netFiles {
		for _, v := range doctorStringConsts(map[string]*ast.File{"n": f}) {
			literals[v] = true
		}
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					literals[s] = true
				}
			}
			return true
		})
	}
	allowed := map[string]bool{}
	for _, k := range doctorOrphanFixes {
		allowed[k] = true
	}
	var orphans []string
	_, fixes := doctorFixServes(t)
	for _, f := range fixes {
		if f.CheckID == "" {
			continue
		}
		reached := false
		for c := range codes {
			if f.ServesCode(c) {
				reached = true
			}
		}
		for c := range literals {
			if f.ServesCode(c) {
				reached = true
			}
		}
		if !reached {
			orphans = append(orphans, f.CheckID)
		}
	}
	sort.Strings(orphans)
	got := map[string]bool{}
	for _, id := range orphans {
		got[id] = true
		if !allowed[id] {
			t.Errorf("typed fix for check %q matches no doctor row code, so it never reaches a row", id)
		}
	}
	for _, id := range doctorOrphanFixes {
		if !got[id] {
			t.Errorf("orphan entry %q now matches a row code, remove it", id)
		}
	}
	if t.Failed() {
		t.Logf("current orphan fixes (%d): %q", len(orphans), orphans)
	}
}
