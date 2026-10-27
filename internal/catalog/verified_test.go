package catalog

import (
	"os"
	"regexp"
	"testing"
)

func TestVerified_SlugsExistInCatalog(t *testing.T) {
	info := VerifiedInfo()
	if info.VerifiedOn == "" || info.Environment == "" {
		t.Fatal("verified.json needs verified_on and environment")
	}
	for _, id := range info.Templates {
		if _, ok := TemplateByID(id); !ok {
			t.Errorf("verified slug %q is not in the catalog", id)
		}
		if !IsVerified(id) {
			t.Errorf("IsVerified(%q) = false", id)
		}
	}
	if IsVerified("definitely-not-a-template") {
		t.Error("unknown slug reported verified")
	}
}

// The status page is the human record of the same run, so the two must not drift.
func TestVerified_MatchesStatusPage(t *testing.T) {
	body, err := os.ReadFile("../../docs/templates-status.md")
	if err != nil {
		t.Skip("docs not present")
	}
	pass := regexp.MustCompile("(?m)^\\| `([^`]+)` \\| pass \\|")
	got := map[string]bool{}
	for _, m := range pass.FindAllStringSubmatch(string(body), -1) {
		got[m[1]] = true
		if !IsVerified(m[1]) {
			t.Errorf("%q passed in docs/templates-status.md but is not in verified.json", m[1])
		}
	}
	for _, id := range VerifiedInfo().Templates {
		if !got[id] {
			t.Errorf("%q is in verified.json but not a pass in docs/templates-status.md", id)
		}
	}
}
