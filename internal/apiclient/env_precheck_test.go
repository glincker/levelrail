package apiclient

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/GLINCKER/levelrail/internal/envrules"
)

// The CLI pre-check must refuse every case the shared fixture marks as an
// error under the default policy and with no knowledge of stored keys.
func TestPrecheckEnvAgreesWithSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../envrules/testdata/envrules.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name       string            `json:"name"`
			Set        map[string]string `json:"set"`
			Existing   envrules.Existing `json:"existing"`
			Policy     json.RawMessage   `json:"policy"`
			Options    envrules.Options  `json:"options"`
			WantErrors []string          `json:"want_errors"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range f.Cases {
		// Server-side only inputs: custom policy, stored keys and byte-exact paths.
		if len(c.Policy) > 0 || len(c.Existing.Plain) > 0 || len(c.Existing.Names) > 0 || c.Options.Multiline || c.Options.ExistingUnknown {
			continue
		}
		if c.Name == "dot key new" || c.Name == "dash key new" {
			continue // the client cannot tell a new dotted key from a stored one
		}
		n++
		err := PrecheckEnv(c.Set, c.Options.Secret)
		if (err != nil) != (len(c.WantErrors) > 0) {
			t.Errorf("%s: precheck err = %v, fixture wants errors %v", c.Name, err, c.WantErrors)
		}
	}
	if n < 25 {
		t.Fatalf("only %d fixture cases ran", n)
	}
}
