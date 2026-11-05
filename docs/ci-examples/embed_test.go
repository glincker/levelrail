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
		if strings.Contains(got, AppPlaceholder) || !strings.Contains(got, "APP: shop") && !strings.Contains(got, "apps deploy shop") {
			t.Errorf("%s: app name not placed where the pipeline names its app", tmpl.ID)
		}
		if strings.Contains(got, "example/shop") || strings.Contains(got, "OWNER/shop") {
			t.Errorf("%s: a repository owner was personalised with the app name", tmpl.ID)
		}
		if tmpl.Auth == AuthToken && !strings.Contains(got, "LEVELRAIL_TOKEN") {
			t.Errorf("%s: token template never reads LEVELRAIL_TOKEN", tmpl.ID)
		}
	}
}

func TestContentRefusesNamesThatCouldInjectIntoAPipeline(t *testing.T) {
	list, _ := Templates()
	for _, name := range []string{"", "Web", "web;rm -rf /", "web\nx", "$(id)", "web\" x"} {
		if _, err := Content(list[0], name); err == nil {
			t.Errorf("Content accepted app name %q", name)
		}
	}
}

func TestTrustCommandsKeepTheOwnerPlaceholder(t *testing.T) {
	list, _ := Templates()
	for _, tmpl := range list {
		if tmpl.Auth != AuthOIDC {
			continue
		}
		got, _ := Content(tmpl, "shop")
		if !strings.Contains(got, "OWNER/REPO") {
			t.Errorf("%s: trust command lost its OWNER/REPO placeholder", tmpl.ID)
		}
	}
}
