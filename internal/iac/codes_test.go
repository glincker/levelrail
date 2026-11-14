package iac

import (
	"context"
	"regexp"
	"testing"
)

func TestChangeCodes(t *testing.T) {
	snake := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
	seen := map[string]bool{}
	for _, c := range AllCodes {
		if !snake.MatchString(c) {
			t.Errorf("code %q is not lowercase snake case", c)
		}
		if seen[c] {
			t.Errorf("code %q is listed twice", c)
		}
		seen[c] = true
	}

	src := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  project: ghost\n  service:\n    build: {type: image, image: \"ghcr.io/acme/web:1\"}\n    port: 3000\n" +
		"---\nversion: 1\nkind: Database\nmetadata: {name: db}\nspec: {project: ghost, engine: postgres, version: \"16\"}\n"
	docs, issues := ParseDocuments([]Source{{Name: "docs.yaml", Data: []byte(src)}})
	if len(issues) > 0 {
		t.Fatalf("parse issues: %v", issues)
	}
	res, issues := Build(docs, Options{})
	if len(issues) > 0 {
		t.Fatalf("build issues: %v", issues)
	}
	st, err := Load(context.Background(), newFake(), res, Options{})
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanFor(st, res, Options{})
	failed := 0
	for _, c := range plan.Changes {
		if c.Action != ActionError {
			continue
		}
		failed++
		if !seen[c.Code] {
			t.Errorf("%s %s failed with unlisted code %q", c.Kind, c.Name, c.Code)
		}
		if c.Code == CodeItemFailed {
			t.Errorf("%s %s fell back to the generic code", c.Kind, c.Name)
		}
	}
	if failed == 0 {
		t.Fatal("expected failing items")
	}
}
