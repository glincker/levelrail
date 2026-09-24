package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAPIReferenceCoversEveryRoute(t *testing.T) {
	reg := regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)"`)
	files, err := filepath.Glob("routes*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no routes files found: %v", err)
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "api-reference.md"))
	if err != nil {
		t.Fatalf("read api reference: %v", err)
	}
	var missing []string
	total := 0
	for _, f := range files {
		src, err := os.ReadFile(f) //nolint:gosec // globbed routes files in the package dir
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reg.FindAllStringSubmatch(string(src), -1) {
			total++
			if !strings.Contains(string(doc), "| "+m[1]+" | "+m[2]+" |") {
				missing = append(missing, m[1]+" "+m[2])
			}
		}
	}
	if total == 0 {
		t.Fatal("parsed zero routes")
	}
	if len(missing) > 0 {
		t.Fatalf("docs/api-reference.md is missing %d registered route(s): %s\nRun `go run ./scripts/gen-api-reference` and commit the result.", len(missing), strings.Join(missing, ", "))
	}
}
