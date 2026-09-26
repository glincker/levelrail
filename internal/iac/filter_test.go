package iac

import (
	"context"
	"testing"
)

func TestFilterProjectKeepsOnlyThatProjectsDocuments(t *testing.T) {
	src := fixture + "---\nversion: 1\nkind: Project\nmetadata: {name: other}\n---\nversion: 1\nkind: App\nmetadata: {name: elsewhere}\nspec:\n  project: other\n  tags: [unrelated]\n  service:\n    build: {type: image, image: x}\n    port: 80\n---\nversion: 1\nkind: Tag\nmetadata: {name: unrelated}\n"
	docs, issues := ParseDocuments([]Source{{Name: "f.yaml", Data: []byte(src)}})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	got := map[string]bool{}
	for _, d := range FilterProject(docs, "shop") {
		got[string(d.Kind)+"/"+d.Name] = true
	}
	for _, want := range []string{"Project/shop", "Environment/production", "Database/main-db", "App/web", "Domain/www.example.com", "LoadBalancer/web", "Pipeline/ci", "AlertRule/high-cpu", "Tag/frontend"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, unwanted := range []string{"Project/other", "App/elsewhere", "Tag/unrelated"} {
		if got[unwanted] {
			t.Errorf("kept %s", unwanted)
		}
	}
}

func TestPruneWithProjectSkipsManagedAppsOfOtherProjects(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git", Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}}
	applyAll(t, f, fixture, opts)
	elsewhere := "version: 1\nkind: App\nmetadata: {name: elsewhere}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	applyAll(t, f, elsewhere, Options{Source: "git"})

	res := mustBuild(t, fixture, Options{Source: "git", Prune: true, Project: "shop"})
	st, err := Load(context.Background(), f, res, Options{Source: "git", Prune: true, Project: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range PlanFor(st, res, Options{Source: "git", Prune: true, Project: "shop"}).Changes {
		if c.Action == ActionDelete {
			t.Fatalf("project scoped prune planned %s", c.Key())
		}
	}
	unscoped := PlanFor(st, res, Options{Source: "git", Prune: true})
	if unscoped.Summary.Delete != 1 {
		t.Fatalf("unscoped prune should see the app outside the files: %+v", unscoped.Summary)
	}
}
