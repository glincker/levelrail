package ciexamples

import (
	"strings"
	"testing"
)

func TestEveryManifestEntryRendersForAnApp(t *testing.T) {
	list, err := Templates()
	if err != nil || len(list) < 5 {
		t.Fatalf("templates = %d, %v", len(list), err)
	}
	seen := map[string]bool{}
	for _, tmpl := range list {
		if seen[tmpl.ID] || tmpl.Target == "" || (tmpl.Auth != AuthOIDC && tmpl.Auth != AuthToken) {
			t.Fatalf("bad manifest entry %+v", tmpl)
		}
		seen[tmpl.ID] = true
		got, err := Content(tmpl, "shop")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, AppPlaceholder) || !strings.Contains(got, "shop") {
			t.Errorf("%s: placeholder not replaced", tmpl.ID)
		}
		if tmpl.Auth == AuthToken && !strings.Contains(got, "LEVELRAIL_TOKEN") {
			t.Errorf("%s: token template never reads LEVELRAIL_TOKEN", tmpl.ID)
		}
	}
}
