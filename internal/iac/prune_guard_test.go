package iac

import (
	"context"
	"strings"
	"testing"
)

func TestPruneRefusesProtectedApp(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git", Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}}
	applyAll(t, f, fixture, opts)
	elsewhere := "version: 1\nkind: App\nmetadata: {name: elsewhere}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	applyAll(t, f, elsewhere, Options{Source: "git"})
	f.route("GET", `/api/v1/apps/([^/]+)/protection`, func(m []string, _ map[string]any) (any, error) {
		return map[string]any{"protected": m[1] == "elsewhere"}, nil
	})

	po := Options{Source: "git", Prune: true}
	res := mustBuild(t, fixture, po)
	st, err := Load(context.Background(), f, res, po)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanFor(st, res, po)
	if plan.Summary.Delete != 0 || plan.Summary.Error != 1 {
		t.Fatalf("summary = %+v", plan.Summary)
	}
	for _, c := range plan.Changes {
		if c.Action == ActionError && !strings.Contains(c.Reason, "protected") {
			t.Fatalf("reason = %q", c.Reason)
		}
	}
}

func TestPlanRefusesJobsBelowRoot(t *testing.T) {
	f := newFake()
	plan := planFor(t, f, refsFixture, Options{BelowRoot: true})
	for _, c := range plan.Changes {
		if c.Kind == KindApp && c.Action == ActionError && strings.Contains(c.Reason, "root-tier") {
			return
		}
	}
	t.Fatalf("jobs were not refused: %+v", plan.Changes)
}
