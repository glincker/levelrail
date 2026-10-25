package iac

import (
	"context"
	"encoding/json"
	"errors"
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

func TestPlaceholdersNeverReadTheServerEnvironment(t *testing.T) {
	t.Setenv("IAC_TEST_SERVER_SECRET", "server-side-value")
	src := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      X: ${{ env.IAC_TEST_SERVER_SECRET }}\n"
	docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(src)}})
	if len(issues) > 0 {
		t.Fatalf("parse issues: %v", issues)
	}
	_, issues = Build(docs, Options{})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "IAC_TEST_SERVER_SECRET is not provided") {
		t.Fatalf("issues = %+v", issues)
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

func TestExportRedactsWhatEnvredactMasks(t *testing.T) {
	f := newFake()
	dbURL := "postgres://u:" + "p@h/db"
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	f.apps["web"] = map[string]any{"name": "web", "image": "x", "port": float64(80), "bind_address": "private", "strategy": "blue-green", "replicas": float64(1),
		"env": map[string]any{"DATABASE_URL": dbURL, "SESSION": jwt, "REGION": "eu"}}
	out, err := Export(context.Background(), f, ExportOptions{IncludeEnvValues: true})
	if err != nil {
		t.Fatal(err)
	}
	j := out.Join()
	if strings.Contains(j, "u:p@h") || strings.Contains(j, "eyJhbGci") || !strings.Contains(j, "REGION: eu") {
		t.Fatalf("export:\n%s", j)
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
	if _, err := Apply(context.Background(), f, res, Options{}, p.Hash); !errors.Is(err, ErrPlanChanged) {
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

// ledgerDB stands in for the stack_resources table: key to owning entry.
type ledgerDB map[string]LedgerEntry

func (db ledgerDB) view(stack string, res []*Resource) *Ledger {
	var own []LedgerEntry
	for _, e := range db {
		if e.StackID == stack {
			own = append(own, e)
		}
	}
	l := NewLedger(stack, stack, "shop", own)
	l.Guard = PruneGuard{MaxPercent: 100, MaxCount: 100}
	for _, ref := range LedgerRefs(res) {
		if e, ok := db[ref.Key]; ok && e.StackID != stack {
			l.Foreign[ref.Key] = e.StackID
		}
	}
	return l
}

// record mirrors the store: a claim on another stack's key is refused.
func (db ledgerDB) record(t *testing.T, stack string, res []*Resource, out ApplyResult) {
	t.Helper()
	u := LedgerChanges(stack, 1, res, out)
	for _, e := range u.Release {
		if db[e.Key].StackID == stack {
			delete(db, e.Key)
		}
	}
	for _, e := range u.Claim {
		if cur, ok := db[e.Key]; ok && cur.StackID != stack {
			t.Fatalf("stack %s claimed %s owned by %s", stack, e.Key, cur.StackID)
		}
		db[e.Key] = e
	}
}

type stackCall struct {
	stack          string
	src            string
	opts           Options
	adopt, mass    bool
	guard          *PruneGuard
	continueOnFail bool
}

func (db ledgerDB) ctx(t *testing.T, c stackCall) ([]*Resource, context.Context) {
	t.Helper()
	c.opts.Source = c.stack
	res := mustBuild(t, c.src, c.opts)
	l := db.view(c.stack, res)
	l.Adopt, l.AllowMassPrune = c.adopt, c.mass
	if c.guard != nil {
		l.Guard = *c.guard
	}
	return res, WithLedger(context.Background(), l)
}

func (db ledgerDB) plan(t *testing.T, f *fakeCP, c stackCall) Plan {
	t.Helper()
	c.opts.Source = c.stack
	res, ctx := db.ctx(t, c)
	st, err := Load(ctx, f, res, c.opts)
	if err != nil {
		t.Fatal(err)
	}
	return PlanFor(st, res, c.opts)
}

func (db ledgerDB) apply(t *testing.T, f *fakeCP, c stackCall) ApplyResult {
	t.Helper()
	c.opts.Source = c.stack
	c.opts.ContinueOnError = c.continueOnFail
	res, ctx := db.ctx(t, c)
	out, err := Apply(ctx, f, res, c.opts, "")
	if err != nil {
		t.Fatal(err)
	}
	db.record(t, c.stack, res, out)
	return out
}

func ledgerFake() *fakeCP {
	f := newFake()
	f.route("DELETE", `/api/v1/environments/([^/]+)`, func(m []string, _ map[string]any) (any, error) { delete(f.envs, m[1]); return nil, nil })
	f.route("DELETE", `/api/v1/tags/([^/]+)`, func(m []string, _ map[string]any) (any, error) { delete(f.tags, m[1]); return nil, nil })
	f.route("DELETE", `/api/v1/apps/([^/]+)/pipelines/([^/]+)`, func(m []string, _ map[string]any) (any, error) {
		delete(f.pipelines[m[1]], m[2])
		return nil, nil
	})
	f.route("DELETE", `/api/v1/apps/([^/]+)/alerts/([^/]+)`, func(m []string, _ map[string]any) (any, error) {
		for n, a := range f.alerts[m[1]] {
			if a["id"] == m[2] {
				delete(f.alerts[m[1]], n)
			}
		}
		return nil, nil
	})
	return f
}

func appDoc(name, image string) string {
	return "version: 1\nkind: App\nmetadata: {name: " + name + "}\nspec:\n  project: shop\n  service:\n    build: {type: image, image: \"" + image + "\"}\n    port: 80\n"
}

const shopProject = "version: 1\nkind: Project\nmetadata: {name: shop}\n---\n"

func errorsByKey(p Plan) map[string]string {
	out := map[string]string{}
	for _, c := range p.Changes {
		if c.Action == ActionError {
			out[c.Key()] = c.Reason
		}
	}
	return out
}

func writesSince(f *fakeCP, n int) []string { return append([]string(nil), f.writes[n:]...) }

func TestLedgerStackCannotOverwriteAnotherStacksResource(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	db.apply(t, f, stackCall{stack: "a", src: shopProject + appDoc("web", "img:a")})
	if db["App/web"].StackID != "a" || db["Project/shop"].StackID != "a" {
		t.Fatalf("ledger after a = %+v", db)
	}

	// Same name from stack b, with and without adopt: always a conflict.
	for _, adopt := range []bool{false, true} {
		before := len(f.writes)
		c := stackCall{stack: "b", src: appDoc("web", "img:b"), adopt: adopt}
		p := db.plan(t, f, c)
		if !strings.HasPrefix(errorsByKey(p)["App/web"], ledgerConflictPrefix) || len(LedgerConflicts(p)) != 1 {
			t.Fatalf("adopt=%v: plan = %+v", adopt, p.Changes)
		}
		out := db.apply(t, f, c)
		if out.Applied != 0 || len(writesSince(f, before)) != 0 {
			t.Fatalf("adopt=%v: b wrote to a's app: %v", adopt, writesSince(f, before))
		}
	}
	if f.apps["web"]["image"] != "img:a" || db["App/web"].StackID != "a" {
		t.Fatalf("a's app changed: %v %+v", f.apps["web"], db["App/web"])
	}

	// Child resources on a's app are a's business too.
	children := "version: 1\nkind: Domain\nmetadata: {name: evil.example.com}\nspec: {app: web}\n---\n" +
		"version: 1\nkind: LoadBalancer\nmetadata: {name: web}\nspec: {algorithm: least_conn}\n"
	p := db.plan(t, f, stackCall{stack: "b", src: children})
	if errs := errorsByKey(p); len(errs) != 2 || !strings.Contains(errs["Domain/evil.example.com"], "owned by stack a") {
		t.Fatalf("children on a foreign app = %+v", p.Changes)
	}
}

func TestLedgerStackCannotPruneAnotherStacksResource(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	db.apply(t, f, stackCall{stack: "a", src: shopProject + appDoc("web", "img:a")})
	db.apply(t, f, stackCall{stack: "b", src: appDoc("api", "img:b") + "---\n" + appDoc("worker", "img:b")})
	// a's app also carries b's managed-by tag: the tag alone grants nothing.
	f.apps["web"]["tags"] = append(stringAny(f.apps["web"]["tags"]), tagManagedPre+"b")

	before := len(f.writes)
	out := db.apply(t, f, stackCall{stack: "b", src: appDoc("api", "img:b"), opts: Options{Prune: true}, mass: true})
	var deletes []string
	for _, r := range out.Results {
		if r.Action == ActionDelete {
			deletes = append(deletes, r.Key())
		}
	}
	if len(deletes) != 1 || deletes[0] != "App/worker" {
		t.Fatalf("b pruned %v", deletes)
	}
	for _, w := range writesSince(f, before) {
		if strings.Contains(w, "/apps/web") || strings.Contains(w, "/projects/") {
			t.Fatalf("b touched a's resources: %v", w)
		}
	}
	if _, ok := f.apps["web"]; !ok {
		t.Fatal("a's app was pruned by b")
	}
	if _, ok := db["App/worker"]; ok {
		t.Fatal("pruned app still in b's ledger")
	}
}

func TestLedgerUnmanagedCollisionIsConflictNotOverwrite(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	applyAll(t, f, shopProject+appDoc("web", "img:hand"), Options{})
	// Even the legacy managed-by tag for this name does not make it owned.
	f.apps["web"]["tags"] = append(stringAny(f.apps["web"]["tags"]), tagManagedPre+"a")
	c := stackCall{stack: "a", src: appDoc("web", "img:stack")}
	p := db.plan(t, f, c)
	if r := errorsByKey(p)["App/web"]; !strings.Contains(r, "no stack owns it") {
		t.Fatalf("unmanaged collision = %+v", p.Changes)
	}
	before := len(f.writes)
	db.apply(t, f, c)
	if len(writesSince(f, before)) != 0 || f.apps["web"]["image"] != "img:hand" {
		t.Fatalf("collision overwrote the app: %v", writesSince(f, before))
	}
	if _, ok := db["App/web"]; ok {
		t.Fatal("refused apply claimed the app")
	}

	c.adopt = true
	p = db.plan(t, f, c)
	if p.Summary.Error != 0 || p.Summary.Update != 1 || !strings.Contains(strings.Join(p.Changes[0].Warnings, ";"), "adopts it into stack a") {
		t.Fatalf("adopt plan = %+v", p.Changes)
	}
	db.apply(t, f, c)
	if db["App/web"].StackID != "a" || f.apps["web"]["image"] != "img:stack" {
		t.Fatalf("adopt did not claim: %+v", db)
	}
}

func TestLedgerScopeProjectIsClaimedOthersAreNot(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	applyAll(t, f, shopProject+"version: 1\nkind: Project\nmetadata: {name: other}\n", Options{})
	src := shopProject + "version: 1\nkind: Project\nmetadata: {name: other}\n"
	p := db.plan(t, f, stackCall{stack: "a", src: src})
	errs := errorsByKey(p)
	if _, bad := errs["Project/shop"]; bad || !strings.HasPrefix(errs["Project/other"], ledgerConflictPrefix) {
		t.Fatalf("scope claim = %+v", p.Changes)
	}
}

func TestLedgerPruneAcrossKindsNeverDatabases(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	secrets := map[string]string{"API_KEY": "k-very-secret", "DB_PASSWORD": "p-very-secret"}
	out := db.apply(t, f, stackCall{stack: "a", src: fixture, opts: Options{Secrets: secrets}})
	if !out.OK() {
		t.Fatalf("first apply = %+v", out.Results)
	}
	for _, k := range []string{"Project/shop", "Environment/shop/production", "Tag/frontend", "Database/main-db", "App/web", "Domain/www.example.com", "LoadBalancer/web", "Pipeline/web/ci", "AlertRule/web/high-cpu"} {
		if db[k].StackID != "a" {
			t.Fatalf("%s not ledgered: %+v", k, db)
		}
	}
	if db["Domain/www.example.com"].LiveID != "web" {
		t.Fatalf("domain live id = %+v", db["Domain/www.example.com"])
	}

	// Only the project is left in the files; drop the database as well.
	before := len(f.writes)
	c := stackCall{stack: "a", src: "version: 1\nkind: Project\nmetadata: {name: shop}\n", opts: Options{Prune: true}, mass: true}
	p := db.plan(t, f, c)
	raw, _ := json.Marshal(p)
	for _, v := range secrets {
		if strings.Contains(string(raw), v) {
			t.Fatal("secret value in plan output")
		}
	}
	got := map[string]Action{}
	for _, ch := range p.Changes {
		got[ch.Key()] = ch.Action
	}
	want := map[string]Action{"Project/shop": ActionNoop, "Database/main-db": ActionNoop, "Environment/shop/production": ActionDelete, "Tag/frontend": ActionDelete,
		"App/web": ActionDelete, "Domain/www.example.com": ActionDelete, "LoadBalancer/web": ActionDelete, "Pipeline/web/ci": ActionDelete, "AlertRule/web/high-cpu": ActionDelete}
	for k, a := range want {
		if got[k] != a {
			t.Fatalf("%s = %q, want %q; plan %+v", k, got[k], a, p.Changes)
		}
	}
	out = db.apply(t, f, c)
	if !out.OK() {
		t.Fatalf("prune apply = %+v", out.Results)
	}
	for _, w := range writesSince(f, before) {
		if strings.Contains(w, "/databases") || strings.HasPrefix(w, "DELETE /api/v1/projects") {
			t.Fatalf("prune touched a database or project: %s", w)
		}
	}
	if _, ok := f.dbs["main-db"]; !ok {
		t.Fatal("database was deleted")
	}
	if len(f.apps) != 0 || len(f.envs) != 0 || len(f.alerts["web"]) != 0 {
		t.Fatalf("leftovers: apps %v envs %v", f.apps, f.envs)
	}
	if db["Database/main-db"].StackID != "a" || len(db) != 2 {
		t.Fatalf("ledger after prune = %+v", db)
	}
}

func TestLedgerMassPruneGuard(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	all := shopProject + appDoc("a1", "x") + "---\n" + appDoc("a2", "x") + "---\n" + appDoc("a3", "x")
	db.apply(t, f, stackCall{stack: "s", src: all})
	keep := shopProject + appDoc("a1", "x")
	guard := PruneGuard{MaxPercent: 40, MaxCount: 10}

	c := stackCall{stack: "s", src: keep, opts: Options{Prune: true}, guard: &guard, continueOnFail: true}
	p := db.plan(t, f, c)
	if !MassPruneBlocked(p) || p.Summary.Delete != 0 || p.Summary.Error != 2 {
		t.Fatalf("guard plan = %+v", p.Summary)
	}
	before := len(f.writes)
	db.apply(t, f, c)
	if len(writesSince(f, before)) != 0 || len(f.apps) != 3 {
		t.Fatalf("guarded prune deleted with ContinueOnError: %v", writesSince(f, before))
	}

	countGuard := PruneGuard{MaxPercent: 100, MaxCount: 1}
	if p := db.plan(t, f, stackCall{stack: "s", src: keep, opts: Options{Prune: true}, guard: &countGuard}); !MassPruneBlocked(p) {
		t.Fatalf("count guard did not trip: %+v", p.Changes)
	}

	c.mass = true
	db.apply(t, f, c)
	if len(f.apps) != 1 {
		t.Fatalf("confirmed prune left %v", f.apps)
	}
}

func TestLedgerGoneResourceIsReleased(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	db.apply(t, f, stackCall{stack: "s", src: shopProject + appDoc("web", "x")})
	delete(f.apps, "web")
	before := len(f.writes)
	db.apply(t, f, stackCall{stack: "s", src: shopProject, opts: Options{Prune: true}})
	if len(writesSince(f, before)) != 0 {
		t.Fatalf("released a gone app with a write: %v", writesSince(f, before))
	}
	if _, ok := db["App/web"]; ok {
		t.Fatal("gone app stayed in the ledger")
	}
}

func TestLedgerChangesClaimNothingFromARefusedPlan(t *testing.T) {
	f, db := ledgerFake(), ledgerDB{}
	applyAll(t, f, shopProject+appDoc("web", "x"), Options{})
	// The tag is new and would be created, but the app collides: nothing is claimed.
	src := "version: 1\nkind: Tag\nmetadata: {name: fresh}\n---\n" + appDoc("web", "y")
	db.apply(t, f, stackCall{stack: "s", src: src})
	if len(db) != 0 {
		t.Fatalf("refused apply claimed %+v", db)
	}
}

func TestLegacySourcePathIgnoresLedger(t *testing.T) {
	f := ledgerFake()
	applyAll(t, f, shopProject+appDoc("web", "x"), Options{Source: "git"})
	p := planFor(t, f, shopProject+appDoc("web", "y"), Options{Source: "git", Prune: true})
	if p.Summary.Error != 0 || p.Summary.Update != 1 {
		t.Fatalf("legacy plan without a ledger = %+v", p.Changes)
	}
}

func TestPruneGuardFromEnv(t *testing.T) {
	t.Setenv("APP_STACK_PRUNE_MAX_PERCENT", "")
	t.Setenv("APP_STACK_PRUNE_MAX_COUNT", "")
	if g := PruneGuardFromEnv(); g.MaxPercent != defaultPruneMaxPercent || g.MaxCount != defaultPruneMaxCount {
		t.Fatalf("defaults = %+v", g)
	}
	t.Setenv("APP_STACK_PRUNE_MAX_PERCENT", "25")
	t.Setenv("APP_STACK_PRUNE_MAX_COUNT", "0")
	if g := PruneGuardFromEnv(); g.MaxPercent != 25 || g.MaxCount != 0 {
		t.Fatalf("env = %+v", g)
	}
	t.Setenv("APP_STACK_PRUNE_MAX_PERCENT", "300")
	if g := PruneGuardFromEnv(); g.MaxPercent != defaultPruneMaxPercent {
		t.Fatalf("out of range percent accepted: %+v", g)
	}
}

func TestSplitLedgerKeyRejectsMalformed(t *testing.T) {
	for _, e := range []LedgerEntry{{Kind: KindApp, Key: "App/"}, {Kind: KindApp, Key: "Tag/web"}, {Kind: KindPipeline, Key: "Pipeline/web"}, {Kind: KindApp, Key: "App/a/b"}} {
		if _, _, _, ok := splitLedgerKey(e); ok {
			t.Fatalf("%+v accepted", e)
		}
	}
	if k, s, n, ok := splitLedgerKey(LedgerEntry{Kind: KindEnvironment, Key: "Environment/shop/prod"}); !ok || k != KindEnvironment || s != "shop" || n != "prod" {
		t.Fatal("environment key not split")
	}
}
