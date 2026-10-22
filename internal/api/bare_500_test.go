package api

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const bare500Allowlist = "testdata/bare_500_allowlist.txt"

var bare500Needles = []string{
	"writeError(w, http.StatusInternalServerError",
	"writeError(w, 500",
}

func loadBare500Allowlist(t *testing.T) map[string]int {
	t.Helper()
	f, err := os.Open(bare500Allowlist)
	if err != nil {
		t.Fatalf("open allow-list: %v", err)
	}
	defer func() { _ = f.Close() }()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			t.Fatalf("bad allow-list line %q, want: <file> <count>", line)
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("bad count in %q: %v", line, err)
		}
		out[parts[0]] = n
	}
	return out
}

// TestBare500AllowList stops new handlers from writing an unclassified 500.
func TestBare500AllowList(t *testing.T) {
	allowed := loadBare500Allowlist(t)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // fixed glob of package sources
		if err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, needle := range bare500Needles {
			got += strings.Count(string(src), needle)
		}
		want := allowed[f]
		if got > want {
			t.Errorf("%s has %d bare 500 writeError calls, allow-list permits %d: use rt.internalError(w, \"context\", err, ...) so the cause is classified, and do not raise the allow-list", f, got, want)
		}
		if got < want {
			t.Errorf("%s has %d bare 500 writeError calls, allow-list says %d: lower its count in %s", f, got, want, bare500Allowlist)
		}
	}
}
