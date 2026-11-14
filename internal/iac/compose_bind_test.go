package iac

import (
	"context"
	"slices"
	"strings"
	"testing"
)

const bindDoc = "version: 1\nkind: Project\nmetadata: {name: store}\n---\nversion: 1\nkind: Environment\nmetadata: {name: prod}\nspec: {project: store}\n---\nversion: 1\nkind: Tag\nmetadata: {name: edge}\n---\n" +
	"version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  project: store\n  environment: prod\n  tags: [edge]\n  compose:\n    path: compose/shop.yaml\n"

func bindApply(t *testing.T, f *fakeCP, body string) ([]*Resource, ApplyResult) {
	t.Helper()
	att := map[string][]byte{"compose/shop.yaml": []byte(body)}
	res, issues := buildCompose(t, bindDoc, att)
	if len(issues) > 0 {
		t.Fatalf("build: %v", issues)
	}
	opts := Options{Attachments: att}
	st, err := Load(context.Background(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanFor(st, res, opts)
	out, err := Apply(context.Background(), f, res, opts, plan.Hash)
	if err != nil {
		t.Fatal(err)
	}
	return res, out
}

func composeResult(t *testing.T, out ApplyResult) ItemResult {
	t.Helper()
	for _, r := range out.Results {
		if r.Kind == KindApp && r.Name == "shop" {
			return r
		}
	}
	t.Fatalf("no App/shop result: %+v", out.Results)
	return ItemResult{}
}

func denyServiceTags(method, path string) bool {
	return method == "POST" && strings.HasPrefix(path, "/api/v1/apps/") && strings.HasSuffix(path, "/tags")
}

func TestComposeBindsEveryServiceApp(t *testing.T) {
	enableComposeExec(t)
	f := newFake()
	res, out := bindApply(t, f, composeA)
	if r := composeResult(t, out); r.Status != StatusApplied {
		t.Fatalf("compose create must apply: %+v", r)
	}
	for _, svc := range []string{"shop-web", "shop-worker"} {
		a := f.apps[svc]
		if a["project_id"] == nil || a["project_id"] == "" || a["environment_id"] == nil || a["environment_id"] == "" || !slices.Contains(stringAny(a["tags"]), any("edge")) {
			t.Fatalf("service %s must carry project, environment and tags: %v", svc, a)
		}
	}
	for _, w := range f.writes {
		if strings.HasPrefix(w, "PUT /api/v1/apps/shop/") || strings.HasPrefix(w, "POST /api/v1/apps/shop/tags") {
			t.Fatalf("the App name has no app row, nothing may target it: %v", f.writes)
		}
	}
	u := LedgerChanges("s1", 1, res, out)
	if !slices.ContainsFunc(u.Claim, func(e LedgerEntry) bool { return e.Key == "App/shop" && e.LiveID == "shop" }) {
		t.Fatalf("the ledger must own the compose App by its name: %+v", u.Claim)
	}
	if got := ComposeCreated(res, out); !slices.Equal(got, []string{"shop"}) {
		t.Fatalf("a first deploy must be listed as created: %v", got)
	}
}

func TestComposeFailedCreateIsRemoved(t *testing.T) {
	enableComposeExec(t)
	f := newFake()
	f.deny = denyServiceTags
	res, out := bindApply(t, f, composeA)
	r := composeResult(t, out)
	if r.Status == StatusApplied || !strings.Contains(r.Error, CodeComposeApplyFailed) {
		t.Fatalf("a failed binding must fail the item with the code: %+v", r)
	}
	n := 0
	for _, w := range f.writes {
		if strings.HasPrefix(w, "DELETE /api/v1/apps/shop/compose") {
			n++
		}
		if strings.Contains(w, "volumes=") {
			t.Fatalf("the teardown must never purge volumes: %v", f.writes)
		}
	}
	if n != 1 || f.apps["shop-web"] != nil || f.apps["shop-worker"] != nil {
		t.Fatalf("the deployed services must be removed once: deletes=%d apps=%v", n, f.apps)
	}
	if u := LedgerChanges("s1", 1, res, out); slices.ContainsFunc(u.Claim, func(e LedgerEntry) bool { return e.Key == "App/shop" }) {
		t.Fatalf("a removed compose App must not be claimed: %+v", u.Claim)
	}
}

func TestComposeFailedUpdateIsKept(t *testing.T) {
	enableComposeExec(t)
	f := newFake()
	if _, out := bindApply(t, f, composeA); composeResult(t, out).Status != StatusApplied {
		t.Fatal("first apply must succeed")
	}
	f.deny = denyServiceTags
	f.writes = nil
	res, out := bindApply(t, f, composeB)
	r := composeResult(t, out)
	if r.Action != ActionUpdate || r.Status == StatusApplied || !strings.Contains(r.Error, CodeComposeApplyFailed) {
		t.Fatalf("a failed redeploy must fail the item with the code: %+v", r)
	}
	for _, w := range f.writes {
		if strings.HasPrefix(w, "DELETE ") {
			t.Fatalf("a failed update must not tear anything down: %v", f.writes)
		}
	}
	if len(f.composeRevs["shop"]) != 2 || f.apps["shop-web"] == nil {
		t.Fatalf("the redeployed state must stay: revs=%d apps=%v", len(f.composeRevs["shop"]), f.apps)
	}
	if got := ComposeCreated(res, out); len(got) != 0 {
		t.Fatalf("an update is never listed as created: %v", got)
	}
}
