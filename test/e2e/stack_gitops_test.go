// Package e2e: Wave 1 exit test for Levelrail stacks. Drives the real
// router over HTTP through create, plan, apply, history, rollback, pin,
// token scoping, idempotency and secret hygiene. No Docker.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

const (
	e2eStackAdmin = "e2e-stack-admin"
	e2eStackPass  = "e2e-stack-correct-horse-battery" //nolint:gosec // test fixture credential
	e2eStackName  = "shop-prod"
	e2eStackToken = "e2e-stack-readonly-token"    //nolint:gosec // test fixture credential
	e2eStackSec   = "s3cr3t-value-0xDEADBEEF-e2e" //nolint:gosec // test fixture, not a credential
	stackKeyV1    = "e2e-ci-run-1"
)

const stackManifestV1 = `version: 1
kind: Project
metadata: {name: shop}
---
version: 1
kind: App
metadata: {name: web}
spec:
  project: shop
  service:
    build: {type: image, image: "ghcr.io/acme/web:1.0.0"}
    port: 3000
    replicas: 1
    env:
      LOG_LEVEL: info
`

const stackManifestV2 = stackManifestV1 + `---
version: 1
kind: App
metadata: {name: worker}
spec:
  project: shop
  service:
    build: {type: image, image: "ghcr.io/acme/worker:1.0.0"}
    port: 4000
    replicas: 1
    env:
      API_KEY: {secretRef: API_KEY}
`

type stackRev struct {
	Number     int    `json:"number"`
	Status     string `json:"status"`
	PlanHash   string `json:"plan_hash"`
	RollbackOf int    `json:"rollback_of"`
	Files      []struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	} `json:"files"`
}

type stackApplyOut struct {
	Revision *stackRev `json:"revision"`
	Replayed bool      `json:"replayed"`
}

type stackPlanOut struct {
	Plan *iac.Plan `json:"plan"`
}

type stackHistOut struct {
	Pinned    int        `json:"pinned_revision"`
	Revisions []stackRev `json:"revisions"`
}

func stackReq(t *testing.T, docs string, extra map[string]any) string {
	t.Helper()
	m := map[string]any{"files": []map[string]string{{"name": "infra/stack.yaml", "content": docs}}}
	for k, v := range extra {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStackGitOps_Live_Wave1Exit(t *testing.T) {
	experimental.Set(experimental.All()...)
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_e2e_stack", Name: "shop", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	secrets := &memSecrets{values: map[string]string{}}
	router := api.NewRouter(discardTestLogger(), &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}, db, api.WithSecretSetter(secrets))
	ts := newE2ETestServer(t, router)
	if err := api.BootstrapAdmin(ctx, db, e2eStackAdmin, e2eStackPass); err != nil {
		t.Fatal(err)
	}
	admin := loginE2EClient(t, ts.URL, e2eStackAdmin, e2eStackPass)
	base := ts.URL + "/api/v1/stacks/" + e2eStackName

	// Every GET body is kept so the secret scan covers all of them.
	var gets []string
	get := func(path string) (int, []byte) {
		t.Helper()
		st, b := requestJSON(t, admin, "GET", ts.URL+path, "")
		gets = append(gets, string(b))
		return st, b
	}
	history := func() stackHistOut {
		t.Helper()
		st, b := get("/api/v1/stacks/" + e2eStackName + "/revisions")
		if st != 200 {
			t.Fatalf("history: %d %s", st, b)
		}
		var h stackHistOut
		if err := json.Unmarshal(b, &h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	apply := func(c *http.Client, docs string, extra map[string]any) (int, stackApplyOut, []byte) {
		t.Helper()
		st, b := requestJSON(t, c, "POST", base+"/apply", stackReq(t, docs, extra))
		var out stackApplyOut
		_ = json.Unmarshal(b, &out)
		return st, out, b
	}
	revision := func(n string) stackRev {
		t.Helper()
		st, b := get("/api/v1/stacks/" + e2eStackName + "/revisions/" + n)
		var r stackRev
		if st != 200 || json.Unmarshal(b, &r) != nil {
			t.Fatalf("revision %s: %d %s", n, st, b)
		}
		return r
	}

	// Create the stack and apply manifest v1.
	if st, b := requestJSON(t, admin, "POST", ts.URL+"/api/v1/stacks", `{"name":"`+e2eStackName+`","project":"shop"}`); st != 201 {
		t.Fatalf("create stack: %d %s", st, b)
	}
	st, v1, body := apply(admin, stackManifestV1, map[string]any{"idempotency_key": stackKeyV1})
	if st != 200 || v1.Revision == nil || v1.Revision.Number != 1 || v1.Revision.Status != "applied" || v1.Replayed {
		t.Fatalf("apply v1: %d %s", st, body)
	}
	if h := history(); len(h.Revisions) != 1 || h.Revisions[0].Number != 1 {
		t.Fatalf("history after v1 = %+v", h)
	}

	// Plan v2 shows the new app and a hash; planning stores nothing.
	// The plan hash covers secret-set items, so plan gets the same secrets as apply.
	st, body = requestJSON(t, admin, "POST", base+"/plan", stackReq(t, stackManifestV2, map[string]any{"secrets": map[string]string{"worker/API_KEY": e2eStackSec}}))
	var plan stackPlanOut
	if st != 200 || json.Unmarshal(body, &plan) != nil || plan.Plan == nil {
		t.Fatalf("plan v2: %d %s", st, body)
	}
	if strings.Contains(string(body), e2eStackSec) || plan.Plan.Hash == "" || plan.Plan.Summary.Create != 1 || !strings.Contains(string(body), "worker") {
		t.Fatalf("plan v2 should create worker with a hash: %s", body)
	}
	if h := history(); len(h.Revisions) != 1 {
		t.Fatalf("plan created a revision: %+v", h)
	}

	// Stale hash (v1's) is refused with no new revision.
	st, _, body = apply(admin, stackManifestV2, map[string]any{"expected_plan_hash": v1.Revision.PlanHash, "secrets": map[string]string{"worker/API_KEY": e2eStackSec}})
	if st != 409 {
		t.Fatalf("stale hash apply = %d, want 409: %s", st, body)
	}
	if h := history(); len(h.Revisions) != 1 {
		t.Fatalf("stale apply created a revision: %+v", h)
	}

	// Fresh hash applies as revision 2.
	st, v2, body := apply(admin, stackManifestV2, map[string]any{"expected_plan_hash": plan.Plan.Hash, "secrets": map[string]string{"worker/API_KEY": e2eStackSec}})
	if st != 200 || v2.Revision == nil || v2.Revision.Number != 2 || v2.Revision.Status != "applied" {
		t.Fatalf("apply v2: %d %s", st, body)
	}

	// Rollback to 1 makes revision 3 whose files equal revision 1.
	st, body = requestJSON(t, admin, "POST", base+"/revisions/1/rollback", "")
	var rb stackApplyOut
	if st != 200 || json.Unmarshal(body, &rb) != nil || rb.Revision == nil || rb.Revision.Number != 3 || rb.Revision.RollbackOf != 1 {
		t.Fatalf("rollback: %d %s", st, body)
	}
	r1, r3 := revision("1"), revision("3")
	if r3.RollbackOf != 1 || len(r1.Files) == 0 || len(r1.Files) != len(r3.Files) {
		t.Fatalf("rollback revision = %+v vs %+v", r3, r1)
	}
	for i := range r1.Files {
		if r1.Files[i] != r3.Files[i] {
			t.Fatalf("rendered file %d differs between revision 1 and 3", i)
		}
	}

	// Idempotency replay returns the stored revision, no new one.
	st, rep, body := apply(admin, stackManifestV1, map[string]any{"idempotency_key": stackKeyV1})
	if st != 200 || !rep.Replayed || rep.Revision == nil || rep.Revision.Number != 1 {
		t.Fatalf("replay: %d %s", st, body)
	}
	if h := history(); len(h.Revisions) != 3 {
		t.Fatalf("replay changed history: %d revisions", len(h.Revisions))
	}

	// Pin 3: apply refused, then unpin.
	if st, b := requestJSON(t, admin, "PUT", base+"/pin", `{"revision":3}`); st != 200 {
		t.Fatalf("pin: %d %s", st, b)
	}
	if st, _, body := apply(admin, stackManifestV2, nil); st != 409 {
		t.Fatalf("apply while pinned = %d, want 409: %s", st, body)
	}
	if st, b := requestJSON(t, admin, "DELETE", base+"/pin", ""); st != 200 {
		t.Fatalf("unpin: %d %s", st, b)
	}
	if h := history(); h.Pinned != 0 || len(h.Revisions) != 3 {
		t.Fatalf("after unpin = %+v", h)
	}

	// Read-only token: plan and history yes, apply no.
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "e2e-stack-ro", Name: "e2e-stack-ro", TokenHash: tokenhash.Sum(e2eStackToken), Abilities: []string{api.AbilityRead}}); err != nil {
		t.Fatal(err)
	}
	ro := &http.Client{Transport: bearerTransport{e2eStackToken}, Timeout: e2eHTTPTimeout}
	if st, b := requestJSON(t, ro, "POST", base+"/plan", stackReq(t, stackManifestV2, nil)); st != 200 {
		t.Fatalf("ro plan: %d %s", st, b)
	}
	if st, b := requestJSON(t, ro, "GET", base+"/revisions", ""); st != 200 {
		t.Fatalf("ro history: %d %s", st, b)
	}
	if st, _, body := apply(ro, stackManifestV2, nil); st != 403 {
		t.Fatalf("ro apply = %d, want 403: %s", st, body)
	}
	if h := history(); len(h.Revisions) != 3 {
		t.Fatalf("ro apply created a revision")
	}

	// The secret must not appear in any revision, diff or stack GET.
	for n := range []string{"1", "2", "3"} {
		num := []string{"1", "2", "3"}[n]
		revision(num)
		get("/api/v1/stacks/" + e2eStackName + "/revisions/" + num + "/diff?against=1")
	}
	get("/api/v1/stacks/" + e2eStackName)
	get("/api/v1/stacks")
	for i, g := range gets {
		if strings.Contains(g, e2eStackSec) {
			t.Fatalf("GET response %d leaks the secret value", i)
		}
	}
}
