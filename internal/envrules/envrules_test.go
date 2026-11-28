package envrules

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

type fixtureCase struct {
	Name         string            `json:"name"`
	Set          map[string]string `json:"set"`
	Existing     Existing          `json:"existing"`
	Policy       json.RawMessage   `json:"policy"`
	Options      Options           `json:"options"`
	WantErrors   []string          `json:"want_errors"`
	WantWarnings []string          `json:"want_warnings"`
}

func codes(is []Issue) []string {
	out := []string{}
	for _, i := range is {
		out = append(out, string(i.Code))
	}
	slices.Sort(out)
	return out
}

// TestFixture runs the shared fixture that the API handler and CLI tests also read.
func TestFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/envrules.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []fixtureCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) < 40 {
		t.Fatalf("fixture has %d cases, want at least 40", len(f.Cases))
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			p := DefaultPolicy()
			if len(c.Policy) > 0 {
				if err := json.Unmarshal(c.Policy, &p); err != nil {
					t.Fatal(err)
				}
			}
			res := Validate(c.Set, c.Existing, p, c.Options)
			want := append([]string{}, c.WantErrors...)
			slices.Sort(want)
			if got := codes(res.Errors); !slices.Equal(got, want) {
				t.Errorf("errors = %v, want %v", got, want)
			}
			wantW := append([]string{}, c.WantWarnings...)
			slices.Sort(wantW)
			if got := codes(res.Warnings); !slices.Equal(got, wantW) {
				t.Errorf("warnings = %v, want %v", got, wantW)
			}
		})
	}
}

func TestValidateKeysOnlyRefusesUnusableNames(t *testing.T) {
	if msg := ValidateKeys([]string{"old.legacy-key", "OK"}).Err(); msg != "" {
		t.Fatalf("legacy name must stay removable, got %q", msg)
	}
	if ValidateKeys([]string{""}).Err() == "" || ValidateKeys([]string{"A=B"}).Err() == "" {
		t.Fatal("empty and = keys must be refused")
	}
}

func TestMessagesNeverContainValues(t *testing.T) {
	res := Validate(map[string]string{"LD_PRELOAD": "supersecretvalue", "A": "x\x00supersecretvalue"}, Existing{}, DefaultPolicy(), Options{})
	for _, i := range append(res.Errors, res.Warnings...) {
		if i.Message == "" || contains(i.Message, "supersecretvalue") {
			t.Fatalf("message leaks value or is empty: %q", i.Message)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
