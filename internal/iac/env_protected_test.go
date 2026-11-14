package iac

import (
	"strings"
	"testing"
)

func TestPlanRefusesBindIntoProtectedEnvironment(t *testing.T) {
	doc := strings.Replace(fixture, "spec: {project: shop}\n---\nversion: 1\nkind: Tag", "spec: {project: shop, protected: true}\n---\nversion: 1\nkind: Tag", 1)
	plan := planFor(t, newFake(), doc, Options{})
	for _, c := range plan.Changes {
		if c.Kind == KindApp && c.Name == "web" {
			if c.Action != ActionError || c.Code != CodeEnvironmentProtected || !strings.Contains(c.Reason, "production") {
				t.Fatalf("web = %+v", c)
			}
			return
		}
	}
	t.Fatal("no App/web change in plan")
}
