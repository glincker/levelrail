// Package execgate holds a test that fails when non-test Go code starts a
// host process from a file that has not been reviewed and listed in
// allowlist.txt.
package execgate

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".claude": true, "docs-local": true, "vendor": true,
}

// spawnSelectors are package-qualified calls that start a process.
var spawnSelectors = map[string]map[string]bool{
	"os":      {"StartProcess": true},
	"syscall": {"Exec": true, "ForkExec": true, "StartProcess": true},
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

func loadAllowlist(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, reason, ok := strings.Cut(line, "|")
		path, reason = strings.TrimSpace(path), strings.TrimSpace(reason)
		if !ok || path == "" || reason == "" {
			t.Fatalf("allowlist line %q needs '<path> | <reason>'", line)
		}
		out[path] = reason
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// spawns returns a description of every process-starting construct in the file.
func spawns(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var found []string
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "os/exec" {
			found = append(found, "imports os/exec")
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && spawnSelectors[id.Name][sel.Sel.Name] {
			found = append(found, id.Name+"."+sel.Sel.Name)
		}
		return true
	})
	return found
}

func TestNoUnreviewedProcessSpawning(t *testing.T) {
	root := repoRoot(t)
	allow := loadAllowlist(t)
	seen := map[string]bool{}
	scanned := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || (d.Name() == "web" && filepath.Dir(path) == root)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		found := spawns(t, path)
		if len(found) == 0 {
			return nil
		}
		seen[rel] = true
		if _, ok := allow[rel]; !ok {
			t.Errorf("%s starts a host process (%s) but is not in test/execgate/allowlist.txt; "+
				"docker and BuildKit must use their APIs, and any real need must be reviewed and listed with a reason", rel, strings.Join(found, ", "))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 200 {
		t.Fatalf("scanned only %d Go files, the walk has drifted", scanned)
	}
	for path := range allow {
		if !seen[path] {
			t.Errorf("allowlist entry %s no longer starts a process, remove it", path)
		}
	}
}
