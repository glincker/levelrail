package iac

import (
	"encoding/json"
	"strings"
	"testing"
)

const refsFixture = `version: 1
kind: Database
metadata: {name: main}
spec: {engine: postgres, version: "16"}
---
version: 1
kind: App
metadata: {name: web}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/web:1"}
    port: 3000
    env:
      DATABASE_URL: {from: {database: main, field: url}}
      PGPASSWORD: {from: main.password}
    jobs:
      - name: nightly
        schedule: "0 3 * * *"
        command: [node, nightly.js]
        timeout: 10m
        memory: 256Mi
`

func TestDatabaseRefsAndJobsApplyIdempotent(t *testing.T) {
	f := newFake()
	out := applyAll(t, f, refsFixture, Options{})
	if !out.OK() {
		t.Fatalf("apply = %+v", out)
	}
	de, _ := f.apps["web"]["database_env"].(map[string]any)
	if len(de) != 2 || de["DATABASE_URL"].(map[string]any)["field"] != "url" || de["PGPASSWORD"].(map[string]any)["field"] != "password" {
		t.Fatalf("connections = %v", de)
	}
	if j := f.jobs["web"]["nightly"]; j == nil || j["timeout_seconds"] != float64(600) || j["memory_limit_mb"] != float64(256) {
		t.Fatalf("job = %v", f.jobs["web"])
	}
	again := planFor(t, f, refsFixture, Options{})
	if again.Pending() {
		t.Fatalf("not idempotent: %+v", again.Changes)
	}
}

func TestDatabaseRefPlanNeverHoldsValues(t *testing.T) {
	f := newFake()
	f.dbs["main"] = map[string]any{"name": "main", "engine": "postgres", "version": "16"}
	plan := planFor(t, f, refsFixture, Options{})
	raw, _ := json.Marshal(plan)
	if !strings.Contains(string(raw), `"main.password"`) || !strings.Contains(string(raw), `"dependsOn":["Database/main"]`) {
		t.Fatalf("plan does not show the reference and its dependency: %s", raw)
	}
}

func TestDatabaseRefIssues(t *testing.T) {
	base := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      DB: "
	for name, c := range map[string]struct{ val, want string }{
		"bad field":    {"{from: {database: main, field: token}}", "field"},
		"bare string":  {"{from: main}", "<database>.<field>"},
		"extra key":    {"{from: {database: main, value: x}}", "value"},
		"vault refuse": {"{vault: {path: a, key: b}}", "vault"},
	} {
		t.Run(name, func(t *testing.T) {
			docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(base + c.val + "\n")}})
			if len(issues) == 0 {
				_, issues = Build(docs, Options{})
			}
			var all strings.Builder
			for _, is := range issues {
				all.WriteString(is.Path + " " + is.Message + "\n")
			}
			if !strings.Contains(all.String(), c.want) {
				t.Fatalf("issues = %v, want %q", issues, c.want)
			}
		})
	}
}

func TestDatabaseRefUnknownDatabaseFailsPlan(t *testing.T) {
	src := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      DB: {from: {database: nope}}\n"
	plan := planFor(t, newFake(), src, Options{})
	if plan.Summary.Error != 1 || !strings.Contains(plan.Changes[0].Reason, `database "nope"`) {
		t.Fatalf("plan = %+v", plan.Changes)
	}
}
