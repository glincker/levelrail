package api

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestAppRoutesUseResourceScopedGuard(t *testing.T) {
	reg := regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) (/api/v1/apps/\{name\}[^"]*)", rt\.requireAbility\(`)
	files, err := filepath.Glob("routes*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no routes files found: %v", err)
	}
	for _, f := range files {
		src, err := os.ReadFile(f) //nolint:gosec // globbed routes files in the package dir
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reg.FindAllStringSubmatch(string(src), -1) {
			key := m[1] + " " + m[2]
			t.Errorf("%s (%s) uses plain requireAbility; use requireAbilityForResource(..., appResourceFromPath, ...)", key, f)
		}
	}
}
