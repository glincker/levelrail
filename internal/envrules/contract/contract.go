// Package contract runs the shared envrules fixture through any writer's
// validation function, so every writer proves the same accept and reject result.
package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/levelrail/internal/envrules"
)

// Case is one fixture entry.
type Case struct {
	Name         string            `json:"name"`
	Set          map[string]string `json:"set"`
	Existing     envrules.Existing `json:"existing"`
	Policy       json.RawMessage   `json:"policy"`
	Options      envrules.Options  `json:"options"`
	WantErrors   []string          `json:"want_errors"`
	WantWarnings []string          `json:"want_warnings"`
}

// DefaultPolicyOnly is true when the case runs under the seeded default policy.
func (c Case) DefaultPolicyOnly() bool { return len(c.Policy) == 0 }

// Refused is what the fixture expects: at least one error.
func (c Case) Refused() bool { return len(c.WantErrors) > 0 }

// Load reads the shared fixture file.
func Load(t testing.TB) []Case {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Tests run from their own package directory, so walk up to the module root.
	var raw []byte
	for {
		raw, err = os.ReadFile(filepath.Clean(filepath.Join(dir, "internal", "envrules", "testdata", "envrules.json")))
		if err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(err)
		}
		dir = parent
	}
	var f struct {
		Cases []Case `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Cases
}

// Run feeds every case to a writer. The writer reports whether it refused the
// set, or skip=true when the case does not apply to its shape (for example a
// single-key writer and a multi-key case). It fails when fewer than minRan ran.
func Run(t *testing.T, minRan int, writer func(c Case) (refused, skip bool)) {
	t.Helper()
	ran := 0
	for _, c := range Load(t) {
		refused, skip := writer(c)
		if skip {
			continue
		}
		ran++
		if refused != c.Refused() {
			t.Errorf("%s: refused = %v, fixture wants %v", c.Name, refused, c.Refused())
		}
	}
	if ran < minRan {
		t.Fatalf("only %d fixture cases ran, want at least %d", ran, minRan)
	}
}
