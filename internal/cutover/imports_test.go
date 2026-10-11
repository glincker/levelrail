package cutover

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The package that moves traffic must have no way to reach the source
// platform: no platform client, no image mover, no SSH, no raw HTTP.
func TestPackageCannotReachTheSource(t *testing.T) {
	forbidden := []string{
		"internal/platformimport", "internal/imagemove", "internal/datamigrate", "internal/appimport",
		"golang.org/x/crypto/ssh", "os/exec", "net/http", "net",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // glob of this package's own files
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			for _, bad := range forbidden {
				if path == bad || strings.HasSuffix(path, "/"+bad) {
					t.Errorf("%s imports %s", f, path)
				}
			}
		}
	}
}
