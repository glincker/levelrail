package apiclient

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Hand written GET getters must use getJSON. The allowlist file lists
// files not yet migrated and may only shrink.
var handGetter = regexp.MustCompile(`\tvar out [^\n]+\n\terr := c\.do\(ctx, http\.MethodGet, `)

func TestNoNewHandWrittenGetters(t *testing.T) {
	raw, err := os.ReadFile("testdata/getter_allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			allowed[l] = true
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "verbs.go" {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // f comes from Glob of this directory
		if err != nil {
			t.Fatal(err)
		}
		has := handGetter.Match(src)
		switch {
		case has && !allowed[f]:
			t.Errorf("%s has a hand written GET getter, use getJSON", f)
		case !has && allowed[f]:
			t.Errorf("%s is clean, remove it from testdata/getter_allowlist.txt", f)
		}
	}
}
