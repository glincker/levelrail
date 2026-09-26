package iac

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const fixture = `version: 1
kind: Project
metadata: {name: shop}
spec:
  env:
    REGION: eu
---
version: 1
kind: Environment
metadata: {name: production}
spec: {project: shop, protected: true}
---
version: 1
kind: Tag
metadata: {name: frontend}
---
version: 1
kind: Database
metadata: {name: main-db}
spec: {engine: postgres, version: "16", project: shop}
---
version: 1
kind: App
metadata: {name: web}
spec:
  project: shop
  environment: production
  tags: [frontend]
  service:
    build: {type: image, image: "ghcr.io/acme/web:1.2.3"}
    port: 3000
    domains: [app.example.com]
    health:
      readiness: {path: /healthz, interval: 5s, timeout: 2s}
    resources: {memory: 512Mi, cpu: 0.5}
    replicas: 2
    env:
      LOG_LEVEL: info
      API_KEY: {secretRef: API_KEY}
      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
---
version: 1
kind: Domain
metadata: {name: www.example.com}
spec: {app: web}
---
version: 1
kind: LoadBalancer
metadata: {name: web}
spec: {algorithm: least_conn}
---
version: 1
kind: Pipeline
metadata: {name: ci}
spec:
  app: web
  yaml: |
    version: 1
    jobs:
      test:
        image: golang:1.22
        steps:
          - run: go test ./...
---
version: 1
kind: AlertRule
metadata: {name: high-cpu}
spec: {app: web, type: threshold, metric: cpu_percent, comparator: ">", threshold: 90, for: 5m, channel: ops}
`

func mustBuild(t *testing.T, src string, opts Options) []*Resource {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(src)}})
	if len(issues) > 0 {
		t.Fatalf("parse issues: %v", issues)
	}
	res, issues := Build(docs, opts)
	if len(issues) > 0 {
		t.Fatalf("build issues: %v", issues)
	}
	return res
}

func TestBuildAndSchemaIssuesCarryLines(t *testing.T) {
	cases := []struct {
		name, src, want string
		line            int
	}{
		{"unknown field", "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    prot: 1\n", "prot", 7},
		{"bad version", "version: 2\nkind: Tag\nmetadata: {name: a}\n", "version", 1},
		{"unknown kind", "version: 1\nkind: Nope\nmetadata: {name: a}\n", "kind", 2},
		{"missing spec", "version: 1\nkind: Database\nmetadata: {name: db}\n", "spec", 1},
		{"git build", "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: dockerfile, path: ./Dockerfile}\n    port: 80\n", "prebuilt image", 6},
		{"bad pipeline", "version: 1\nkind: Pipeline\nmetadata: {name: ci}\nspec:\n  app: web\n  yaml: |\n    version: 1\n    jobs:\n      a:\n        steps:\n          - run: x\n", "image is required", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			docs, pi := ParseDocuments([]Source{{Name: "f.yaml", Data: []byte(tc.src)}})
			if len(pi) > 0 {
				t.Fatalf("parse: %v", pi)
			}
			_, issues := Build(docs, Options{})
			if len(issues) == 0 {
				t.Fatal("expected issues")
			}
			found := false
			for _, i := range issues {
				if strings.Contains(i.Message+" "+i.Path, tc.want) && (tc.line == 0 || i.Line == tc.line) && i.File == "f.yaml" {
					found = true
				}
			}
			if !found {
				t.Fatalf("want %q at line %d, got %v", tc.want, tc.line, issues)
			}
		})
	}
}

func TestYAMLSyntaxErrorLine(t *testing.T) {
	_, issues := ParseDocuments([]Source{{Name: "x.yaml", Data: []byte("version: 1\nkind: [\n")}})
	if len(issues) != 1 || issues[0].Line == 0 {
		t.Fatalf("issues = %v", issues)
	}
}

func TestSecretValuesRefused(t *testing.T) {
	cases := []struct{ name, env, want string }{
		{"secret key literal", "DB_PASSWORD: hunter2", "looks like a secret"},
		{"token value", "SOMETHING: ghp_abcdefghijklmnopqrstuvwxyz0123456789", "looks like a secret"},
		{"secretRef name mismatch", "TOKEN: {secretRef: OTHER}", "must match"},
		{"vault unsupported", "X: {vault: {path: a, key: b}}", "not supported"},
		{"missing var", "REGION: ${{ env.MISSING }}", "MISSING"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      " + tc.env + "\n"
			docs, _ := ParseDocuments([]Source{{Name: "f.yaml", Data: []byte(src)}})
			_, issues := Build(docs, Options{})
			if len(issues) == 0 || !strings.Contains(issues[0].Message, tc.want) {
				t.Fatalf("want %q, got %v", tc.want, issues)
			}
			if issues[0].Line == 0 {
				t.Fatalf("issue has no line: %v", issues[0])
			}
			for _, i := range issues {
				if strings.Contains(i.Message, "hunter2") {
					t.Fatalf("issue leaks the value: %v", i)
				}
			}
		})
	}
}

func TestPlaceholderVarsResolveAndSecretRefsAllowed(t *testing.T) {
	src := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      REGION: ${{ env.REGION }}\n      DB_PASSWORD: ${{ env.DB_PASSWORD }}\n      API_KEY: {secretRef: API_KEY}\n"
	res := mustBuild(t, src, Options{Vars: map[string]string{"REGION": "eu", "DB_PASSWORD": "from-ci"}})
	env := res[0].Fields["env"].(map[string]any)
	if env["REGION"] != "eu" || env["DB_PASSWORD"] != "from-ci" {
		t.Fatalf("env = %v", env)
	}
	if refs := res[0].SecretRefs; len(refs) != 1 || refs[0] != "API_KEY" {
		t.Fatalf("refs = %v", refs)
	}
}

func TestDuplicatesAndDomainConflicts(t *testing.T) {
	dup := "version: 1\nkind: Tag\nmetadata: {name: a}\n---\nversion: 1\nkind: Tag\nmetadata: {name: a}\n"
	docs, _ := ParseDocuments([]Source{{Name: "d.yaml", Data: []byte(dup)}})
	if _, issues := Build(docs, Options{}); len(issues) == 0 || !strings.Contains(issues[0].Message, "declared twice") {
		t.Fatalf("issues = %v", issues)
	}
	conflict := "version: 1\nkind: Domain\nmetadata: {name: x.example.com}\nspec: {app: a}\n---\nversion: 1\nkind: Domain\nmetadata: {name: x.example.com}\nspec: {app: b}\n"
	docs, _ = ParseDocuments([]Source{{Name: "d.yaml", Data: []byte(conflict)}})
	if _, issues := Build(docs, Options{}); len(issues) == 0 {
		t.Fatal("expected a conflict issue")
	}
}

func planFor(t *testing.T, f *fakeCP, src string, opts Options) Plan {
	t.Helper()
	res := mustBuild(t, src, opts)
	st, err := Load(context.Background(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := PlanFor(st, res, opts)
	return p
}

func TestPlanCreateOrdersByDependency(t *testing.T) {
	f := newFake()
	p := planFor(t, f, fixture, Options{})
	if p.Summary.Create != 9 || p.Summary.Error != 0 {
		t.Fatalf("summary = %+v", p.Summary)
	}
	rank := map[Kind]int{}
	for i, c := range p.Changes {
		if _, seen := rank[c.Kind]; !seen {
			rank[c.Kind] = i
		}
	}
	order := []Kind{KindProject, KindEnvironment, KindTag, KindDatabase, KindApp, KindDomain, KindLoadBalancer, KindPipeline, KindAlertRule}
	for i := 1; i < len(order); i++ {
		if rank[order[i-1]] > rank[order[i]] {
			t.Fatalf("%s planned after %s: %+v", order[i-1], order[i], p.Changes)
		}
	}
	if f.writeCount() != 0 {
		t.Fatalf("planning wrote: %v", f.writes)
	}
}

func TestPlanNeverShowsEnvValues(t *testing.T) {
	f := newFake()
	p := planFor(t, f, fixture, Options{})
	for _, c := range p.Changes {
		for _, fc := range c.Fields {
			if strings.HasPrefix(fc.Path, "env.") && (fc.New != hiddenValue) {
				t.Fatalf("env value shown: %+v", fc)
			}
			if strings.Contains(fc.New, "info") && strings.HasPrefix(fc.Path, "env.") {
				t.Fatalf("leak: %+v", fc)
			}
		}
	}
}

func applyAll(t *testing.T, f *fakeCP, src string, opts Options) ApplyResult {
	t.Helper()
	res := mustBuild(t, src, opts)
	out, err := Apply(context.Background(), f, res, opts, "")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestApplyThenReplanIsEmpty(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git", Secrets: map[string]string{"API_KEY": "k1", "web/DB_PASSWORD": "p1"}}
	out := applyAll(t, f, fixture, opts)
	if !out.OK() || out.Applied != 9 {
		t.Fatalf("apply = %+v", out)
	}
	if !f.secrets["web"]["API_KEY"] || !f.secrets["web"]["DB_PASSWORD"] {
		t.Fatalf("secrets not stored: %v", f.secrets)
	}
	again := planFor(t, f, fixture, Options{Source: "git"})
	if again.Pending() {
		for _, c := range again.Changes {
			if c.Action != ActionNoop {
				t.Errorf("not idempotent: %s %s %+v", c.Action, c.Key(), c.Fields)
			}
		}
	}
	f.writes = nil
	if o := applyAll(t, f, fixture, Options{Source: "git"}); o.Applied != 0 || f.writeCount() != 0 {
		t.Fatalf("second apply wrote: %v", f.writes)
	}
}

func TestExportApplyExportRoundTrip(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	ctx := context.Background()
	first, err := Export(ctx, f, ExportOptions{IncludeEnvValues: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) == 0 {
		t.Fatal("empty export")
	}
	joined := first.Join()
	if !strings.Contains(joined, "secretRef: API_KEY") || !strings.Contains(joined, "secretRef: DB_PASSWORD") {
		t.Fatalf("export:\n%s", joined)
	}
	for _, kind := range []string{"kind: Project", "kind: Environment", "kind: App", "kind: Database", "kind: LoadBalancer", "kind: Pipeline", "kind: AlertRule", "kind: Tag"} {
		if !strings.Contains(joined, kind) {
			t.Errorf("export lacks %s", kind)
		}
	}
	f.writes = nil
	res := applyAll(t, f, joined, Options{})
	if res.Applied != 0 || f.writeCount() != 0 {
		t.Fatalf("applying an export changed things: %v\n%+v", f.writes, res.Plan.Changes)
	}
	second, err := Export(ctx, f, ExportOptions{IncludeEnvValues: true})
	if err != nil {
		t.Fatal(err)
	}
	if second.Join() != joined {
		t.Fatalf("export not stable:\n%s\n---\n%s", joined, second.Join())
	}
}

func TestExportScopesAndRedaction(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	other := "version: 1\nkind: App\nmetadata: {name: other}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	applyAll(t, f, other, Options{})
	one, err := Export(context.Background(), f, ExportOptions{App: "web", IncludeEnvValues: false})
	if err != nil {
		t.Fatal(err)
	}
	j := one.Join()
	if strings.Contains(j, "name: other") || strings.Contains(j, "kind: Project") {
		t.Fatalf("app export not scoped:\n%s", j)
	}
	if strings.Contains(j, "LOG_LEVEL: info") || !strings.Contains(j, "${{ env.LOG_LEVEL }}") {
		t.Fatalf("values not redacted:\n%s", j)
	}
	proj, err := Export(context.Background(), f, ExportOptions{Project: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(proj.Join(), "name: other") {
		t.Fatal("project export included an app from outside the project")
	}
}

func TestExportNeverWritesSecretLookingValues(t *testing.T) {
	f := newFake()
	f.apps["web"] = map[string]any{"name": "web", "image": "x", "port": float64(80), "bind_address": "private", "strategy": "blue-green", "replicas": float64(1),
		"env": map[string]any{"API_TOKEN": "sk-abcdefghijklmnopqrstuvwxyz", "OK": "fine"}}
	out, err := Export(context.Background(), f, ExportOptions{IncludeEnvValues: true})
	if err != nil {
		t.Fatal(err)
	}
	j := out.Join()
	if strings.Contains(j, "sk-abcdef") || !strings.Contains(j, "${{ env.API_TOKEN }}") || !strings.Contains(j, "OK: fine") {
		t.Fatalf("export:\n%s", j)
	}
	if len(out.Warnings) == 0 {
		t.Fatal("no warning for the redacted secret")
	}
}

func TestPruneOnlyTouchesManaged(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Source: "git", Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	hand := "version: 1\nkind: App\nmetadata: {name: handmade}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	applyAll(t, f, hand, Options{})
	stray := "version: 1\nkind: App\nmetadata: {name: stray}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	applyAll(t, f, stray, Options{Source: "git"})
	applyAll(t, f, stray, Options{Source: "other-source"})

	p := planFor(t, f, fixture, Options{Source: "git", Prune: true})
	var deletes []string
	for _, c := range p.Changes {
		if c.Action == ActionDelete {
			deletes = append(deletes, c.Key())
		}
	}
	if len(deletes) != 1 || deletes[0] != "App/stray" {
		t.Fatalf("deletes = %v", deletes)
	}

	if _, ok := f.apps["handmade"]; !ok {
		t.Fatal("hand made app vanished")
	}
	res := mustBuild(t, fixture, Options{Source: "git", Prune: true})
	if _, err := Apply(context.Background(), f, res, Options{Source: "git", Prune: true}, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.apps["stray"]; ok {
		t.Fatal("managed stray app was not pruned")
	}
	if _, ok := f.apps["handmade"]; !ok {
		t.Fatal("prune deleted a hand made app")
	}
}

func TestPruneNeedsASourceAndKeepsExtrasWithoutPrune(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Source: "git", Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	f.apps["web"]["domains"] = []any{"app.example.com", "www.example.com", "hand.example.com"}
	p := planFor(t, f, fixture, Options{Source: "git"})
	if p.Pending() {
		t.Fatalf("an extra live domain must not be a change without prune: %+v", p.Changes)
	}
	var kept bool
	for _, c := range p.Changes {
		for _, k := range c.Kept {
			kept = kept || strings.Contains(k.Path, "hand.example.com")
		}
	}
	if !kept {
		t.Fatal("extra domain not reported as kept")
	}
	pruned := planFor(t, f, fixture, Options{Source: "git", Prune: true})
	if pruned.Summary.Update != 1 {
		t.Fatalf("prune should remove the extra domain from a managed app: %+v", pruned.Changes)
	}
	noSource := planFor(t, f, fixture, Options{Prune: true})
	if noSource.Summary.Delete != 0 || noSource.Summary.Update != 0 {
		t.Fatalf("prune without a source must do nothing destructive: %+v", noSource.Changes)
	}
}

func TestDeniedItemsAreReportedPerItem(t *testing.T) {
	f := newFake()
	f.deny = func(method, path string) bool { return method == http.MethodPost && strings.HasSuffix(path, "/alerts") }
	opts := Options{ContinueOnError: true, Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}}
	out := applyAll(t, f, fixture, opts)
	var denied []string
	for _, r := range out.Results {
		if r.Status == StatusDenied {
			denied = append(denied, r.Key())
		}
	}
	if len(denied) != 1 || denied[0] != "AlertRule/web/high-cpu" {
		t.Fatalf("denied = %v; results = %+v", denied, out.Results)
	}
	if out.Applied != 8 || out.OK() {
		t.Fatalf("applied = %d ok = %v", out.Applied, out.OK())
	}
}

func TestDeniedStopsWithoutContinueOnError(t *testing.T) {
	f := newFake()
	f.deny = func(method, path string) bool { return method == http.MethodPost && path == "/api/v1/databases" }
	out := applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	if out.Skipped == 0 || out.Failed != 1 {
		t.Fatalf("apply = failed %d skipped %d", out.Failed, out.Skipped)
	}
	if _, ok := f.apps["web"]; ok {
		t.Fatal("later items ran after a failure")
	}
}

func TestReadDeniedBlocksApplyUnlessContinuing(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	f.deny = func(method, path string) bool { return method == http.MethodGet && path == "/api/v1/apps/web" }
	f.writes = nil
	res := mustBuild(t, fixture, Options{})
	out, err := Apply(context.Background(), f, res, Options{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Plan.Summary.Error == 0 || f.writeCount() != 0 {
		t.Fatalf("plan = %+v writes = %v", out.Plan.Summary, f.writes)
	}
	var sawDenied bool
	for _, c := range out.Plan.Changes {
		sawDenied = sawDenied || (c.Denied && c.Key() == "App/web")
	}
	if !sawDenied {
		t.Fatalf("no denied app item: %+v", out.Plan.Changes)
	}
}

func TestPlanHashGuardsStaleApply(t *testing.T) {
	f := newFake()
	res := mustBuild(t, fixture, Options{})
	st, _ := Load(context.Background(), f, res, Options{})
	p := PlanFor(st, res, Options{})
	f.projects["px"] = map[string]any{"id": "px", "name": "shop"}
	if _, err := Apply(context.Background(), f, res, Options{}, p.Hash); err != ErrPlanChanged {
		t.Fatalf("err = %v", err)
	}
	if f.writeCount() != 0 {
		t.Fatalf("stale apply wrote: %v", f.writes)
	}
}

func TestUpdateChangesFieldsAndKeepsUnmanagedSettings(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	f.apps["web"]["vault_env"] = map[string]any{"V": map[string]any{"path": "a", "key": "b"}}
	changed := strings.Replace(fixture, "1.2.3", "1.3.0", 1)
	changed = strings.Replace(changed, "replicas: 2", "replicas: 3", 1)
	p := planFor(t, f, changed, Options{})
	var app *Change
	for i := range p.Changes {
		if p.Changes[i].Kind == KindApp {
			app = &p.Changes[i]
		}
	}
	if app == nil || app.Action != ActionUpdate {
		t.Fatalf("plan = %+v", p.Changes)
	}
	paths := map[string]FieldChange{}
	for _, fc := range app.Fields {
		paths[fc.Path] = fc
	}
	if paths["image"].Old != "ghcr.io/acme/web:1.2.3" || paths["image"].New != "ghcr.io/acme/web:1.3.0" || paths["replicas"].New != "3" || len(paths) != 2 {
		t.Fatalf("diff = %+v", app.Fields)
	}
	if len(app.Warnings) == 0 {
		t.Fatal("unmanaged vault env not warned about")
	}
	applyAll(t, f, changed, Options{})
	if f.apps["web"]["vault_env"] == nil || f.apps["web"]["image"] != "ghcr.io/acme/web:1.3.0" {
		t.Fatalf("app = %v", f.apps["web"])
	}
}

func TestNoDeployAndEnvChangeRestart(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	changed := strings.Replace(fixture, "LOG_LEVEL: info", "LOG_LEVEL: debug", 1)
	f.writes = nil
	applyAll(t, f, changed, Options{NoDeploy: true})
	if hasWrite(f, "POST /api/v1/apps/web/restart") {
		t.Fatal("no-deploy restarted the app")
	}
	changed2 := strings.Replace(changed, "LOG_LEVEL: debug", "LOG_LEVEL: warn", 1)
	f.writes = nil
	applyAll(t, f, changed2, Options{})
	if !hasWrite(f, "POST /api/v1/apps/web/restart") {
		t.Fatalf("env change did not restart: %v", f.writes)
	}
}

func TestMissingReferencesFailThePlan(t *testing.T) {
	src := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  project: ghost\n  service:\n    build: {type: image, image: x}\n    port: 80\n"
	p := planFor(t, newFake(), src, Options{})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "ghost") {
		t.Fatalf("plan = %+v", p)
	}
	f := newFake()
	res := mustBuild(t, src, Options{})
	out, _ := Apply(context.Background(), f, res, Options{}, "")
	if out.Applied != 0 || f.writeCount() != 0 {
		t.Fatalf("an invalid plan wrote: %v", f.writes)
	}
}

func TestDatabaseEngineChangeRefused(t *testing.T) {
	f := newFake()
	applyAll(t, f, fixture, Options{Secrets: map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}})
	changed := strings.Replace(fixture, "engine: postgres", "engine: mysql", 1)
	p := planFor(t, f, changed, Options{})
	if p.Summary.Error != 1 {
		t.Fatalf("summary = %+v", p.Summary)
	}
}

func TestPublishedSchemaMatchesMerged(t *testing.T) {
	raw, err := SchemaJSON()
	if err != nil || !strings.Contains(string(raw), "secretRef") || !strings.Contains(string(raw), `"service"`) {
		t.Fatalf("schema: %v", err)
	}
}

func TestPruneRemovesLastSecretReference(t *testing.T) {
	f := newFake()
	secrets := map[string]string{"API_KEY": "k", "DB_PASSWORD": "p"}
	applyAll(t, f, fixture, Options{Source: "git", Secrets: secrets})
	if refs, _ := f.apps["web"]["secret_env"].([]any); len(refs) == 0 {
		t.Fatalf("fixture app has no secret references: %v", f.apps["web"])
	}
	stripped := strings.Replace(fixture, "      API_KEY: {secretRef: API_KEY}\n", "", 1)
	stripped = strings.Replace(stripped, "      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}\n", "", 1)
	applyAll(t, f, stripped, Options{Source: "git", Prune: true})
	if refs, _ := f.apps["web"]["secret_env"].([]any); len(refs) != 0 {
		t.Fatalf("prune left secret references behind: %v", f.apps["web"]["secret_env"])
	}
}
