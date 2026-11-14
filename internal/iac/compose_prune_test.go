package iac

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const envDoc = "version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  compose:\n    path: compose/shop.yaml\n    env:\n      LOG_LEVEL: %s\n"

func envPlan(t *testing.T, f *fakeCP, level string, applied map[string]string) Plan {
	t.Helper()
	att := map[string][]byte{"compose/shop.yaml": []byte(composeA)}
	res, issues := buildCompose(t, strings.Replace(envDoc, "%s", level, 1), att)
	if len(issues) > 0 {
		t.Fatalf("build: %v", issues)
	}
	opts := Options{Attachments: att, ComposeEnvApplied: applied}
	st, err := Load(context.Background(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	return PlanFor(st, res, opts)
}

func TestComposeDriftFromDeployedRevision(t *testing.T) {
	enableComposeExec(t)
	f := newFake()
	res, plan, opts := composePlan(t, f, composeA, Options{})
	if _, err := Apply(context.Background(), f, res, opts, plan.Hash); err != nil {
		t.Fatal(err)
	}
	if _, plan, _ = composePlan(t, f, composeA, Options{}); plan.Changes[0].Action != ActionNoop {
		t.Fatalf("unchanged must show no drift: %+v", plan.Changes[0])
	}
	// An out of band redeploy from the dashboard stores a new revision body.
	f.composeRevs["shop"] = append(f.composeRevs["shop"], map[string]any{"revision": 2, "compose": composeB, "profiles": []string{}})
	_, plan, _ = composePlan(t, f, composeA, Options{})
	c := plan.Changes[0]
	if c.Action != ActionUpdate || len(c.Fields) != 1 || c.Fields[0].Path != "compose.digest" {
		t.Fatalf("an out of band redeploy must show as drift: %+v", c)
	}
}

func TestComposeEnvDriftHidesValues(t *testing.T) {
	enableComposeExec(t)
	const canary = "lvl-canary-7f3a"
	f := newFake()
	f.composeRevs = map[string][]map[string]any{"shop": {{"revision": 1, "compose": composeA, "profiles": []string{}}}}
	applied := map[string]string{"shop": composeEnvDigest(map[string]string{"LOG_LEVEL": canary})}
	if p := envPlan(t, f, canary, applied); p.Changes[0].Action != ActionNoop {
		t.Fatalf("env equal to the last applied must be a noop: %+v", p.Changes[0])
	}
	p := envPlan(t, f, canary+"-new", applied)
	c := p.Changes[0]
	if c.Action != ActionUpdate || len(c.Fields) != 1 || c.Fields[0].Path != "compose.env" {
		t.Fatalf("an env change must show as an update: %+v", c)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), canary) {
		t.Fatalf("an env value leaked into the plan: %s", raw)
	}
	if p := envPlan(t, f, canary, nil); p.Changes[0].Action != ActionUpdate {
		t.Fatal("without an applied record, declared env must still redeploy")
	}
	got := ComposeEnvDigests([]Source{{Name: "shop.yaml", Data: []byte(strings.Replace(envDoc, "%s", canary, 1))}, {Name: "compose/shop.yaml", Data: []byte(composeA)}}, "")
	if got["shop"] != applied["shop"] {
		t.Fatalf("revision env digest must match the declared one: %v", got)
	}
}

func composePruneSetup(t *testing.T, protected bool) (*fakeCP, context.Context) {
	t.Helper()
	enableComposeExec(t)
	f := newFake()
	f.composeRevs = map[string][]map[string]any{
		"shop":  {{"revision": 1, "compose": composeA, "profiles": []string{}}},
		"other": {{"revision": 1, "compose": composeA, "profiles": []string{}}},
	}
	f.route("DELETE", `/api/v1/apps/([^/?]+)/compose(?:\?.*)?`, func(m []string, _ map[string]any) (any, error) {
		delete(f.composeRevs, m[1])
		return map[string]any{"app_id": m[1]}, nil
	})
	f.route("GET", `/api/v1/apps/([^/]+)/protection`, func(m []string, _ map[string]any) (any, error) {
		return map[string]any{"protected": protected && m[1] == "shop"}, nil
	})
	l := NewLedger("s1", "web", "", []LedgerEntry{{StackID: "s1", Kind: KindApp, Key: "App/shop", LiveID: "shop"}})
	l.Foreign["App/other"] = "billing"
	l.AllowMassPrune = true
	return f, WithLedger(context.Background(), l)
}

func prunePlan(ctx context.Context, t *testing.T, f *fakeCP, opts Options) Plan {
	t.Helper()
	st, err := Load(ctx, f, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	return PlanFor(st, nil, opts)
}

func TestComposePruneNeedsLedgerPruneAndConfirm(t *testing.T) {
	f, ctx := composePruneSetup(t, false)
	if p := prunePlan(ctx, t, f, Options{ConfirmDestroy: true}); len(p.Changes) != 0 {
		t.Fatalf("prune off must keep the app: %+v", p.Changes)
	}
	p := prunePlan(ctx, t, f, Options{Prune: true})
	if len(p.Changes) != 1 || p.Changes[0].Code != CodeDestroyNotConfirmed || p.Changes[0].Action != ActionError {
		t.Fatalf("a compose delete without confirm_destroy must refuse: %+v", p.Changes)
	}
	opts := Options{Prune: true, ConfirmDestroy: true}
	p = prunePlan(ctx, t, f, opts)
	c := p.Changes[0]
	if len(p.Changes) != 1 || c.Name != "shop" || c.Action != ActionDelete || len(c.Services) != 2 || c.Services[0].Action != ActionDelete {
		t.Fatalf("an owned compose app must plan a delete with service rows: %+v", p.Changes)
	}
	if _, err := Apply(ctx, f, nil, opts, p.Hash); err != nil {
		t.Fatal(err)
	}
	if n := countWrites(f.writes, "DELETE /api/v1/apps/shop/compose"); n != 1 {
		t.Fatalf("the teardown must call the compose delete path once: %v", f.writes)
	}
	for _, w := range f.writes {
		if strings.Contains(w, "other") || strings.Contains(w, "volumes=") {
			t.Fatalf("another stack's app or a volume purge was touched: %v", f.writes)
		}
	}
	if _, kept := f.composeRevs["other"]; !kept {
		t.Fatal("another stack's compose app must stay")
	}
}

func TestComposePruneRefusesProtected(t *testing.T) {
	f, ctx := composePruneSetup(t, true)
	p := prunePlan(ctx, t, f, Options{Prune: true, ConfirmDestroy: true})
	if len(p.Changes) != 1 || p.Changes[0].Code != CodeProtectedResource {
		t.Fatalf("a protected compose app must refuse: %+v", p.Changes)
	}
	if _, err := Apply(ctx, f, nil, Options{Prune: true, ConfirmDestroy: true}, p.Hash); err == nil && slices.Contains(f.writes, "DELETE /api/v1/apps/shop/compose") {
		t.Fatal("a protected compose app must not be torn down")
	}
}

func countWrites(writes []string, want string) int {
	n := 0
	for _, w := range writes {
		if w == want {
			n++
		}
	}
	return n
}

// A Tag still in the files must plan a noop when only its sibling compose App is dropped.
func TestComposePruneKeepsDeclaredTag(t *testing.T) {
	f, _ := composePruneSetup(t, false)
	f.ensureTag("keep")
	l := NewLedger("s1", "web", "", []LedgerEntry{
		{StackID: "s1", Kind: KindApp, Key: "App/shop", LiveID: "shop"},
		{StackID: "s1", Kind: KindTag, Key: "Tag/keep", LiveID: "keep"},
	})
	l.AllowMassPrune = true
	ctx := WithLedger(context.Background(), l)
	res, issues := buildCompose(t, "version: 1\nkind: Tag\nmetadata: {name: keep}\n", nil)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	opts := Options{Prune: true, ConfirmDestroy: true}
	st, err := Load(ctx, f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := PlanFor(st, res, opts)
	for _, c := range p.Changes {
		if c.Kind == KindTag && c.Action != ActionNoop {
			t.Fatalf("a declared tag must be a noop, got %+v", c)
		}
		if c.Kind == KindApp && c.Action != ActionDelete {
			t.Fatalf("the dropped compose app must plan a delete: %+v", c)
		}
	}
	if _, err := Apply(ctx, f, res, opts, p.Hash); err != nil {
		t.Fatal(err)
	}
	for _, w := range f.writes {
		if strings.Contains(w, "/tags/") {
			t.Fatalf("a tag write happened: %v", f.writes)
		}
	}
}
