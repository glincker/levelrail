package setupguide

import (
	"encoding/json"
	"os"
	"testing"
)

// Every catalog step needs its card copy in the dashboard locale, or the page shows raw keys.
func TestEveryStepHasLocaleCopy(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/locales/en/getStarted.json")
	if err != nil {
		t.Skipf("locale file not reachable: %v", err)
	}
	var doc struct {
		Steps map[string]map[string]string `json:"steps"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, m := range catalog {
		block, ok := doc.Steps[string(m.id)]
		if !ok {
			t.Errorf("step %q has no steps.%s block in getStarted.json", m.id, m.id)
			continue
		}
		for _, key := range []string{"title", "what", "why", "cta"} {
			if block[key] == "" {
				t.Errorf("step %q is missing steps.%s.%s", m.id, m.id, key)
			}
		}
	}
}
