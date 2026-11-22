package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The allowlist may only shrink: new handlers use decodeJSON instead of the raw idiom.
func TestDecodeJSONGuardNoNewRawIdiom(t *testing.T) {
	raw, err := os.ReadFile("decode_json_allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			allowed[l] = true
		}
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "decode_json.go" {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // fixed glob in package dir
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		has := strings.Contains(s, "http.MaxBytesReader") && strings.Contains(s, "json.NewDecoder")
		if has && !allowed[f] {
			t.Errorf("%s uses raw MaxBytesReader+json.NewDecoder; use decodeJSON", f)
		}
		if !has && allowed[f] {
			t.Errorf("%s is migrated; remove it from decode_json_allowlist.txt", f)
		}
	}
}
