package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

func stackTestDocs(image string) string {
	return strings.Replace(iacTestDocs, "ghcr.io/acme/web:1.2.3", image, 1)
}

func stackBody(t *testing.T, docs string, extra map[string]any) string {
	t.Helper()
	return iacRequestBody(t, docs, extra)
}

// stackFixture returns a router with an admin session, project "shop" and stack "shop-prod".
func stackFixture(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	return rt, db, stackSeed(t, rt, db)
}

func stackSeed(t *testing.T, rt *Router, db *store.DB) *http.Cookie {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(t.Context(), store.Project{ID: "p-shop", Name: "shop", CreatedAt: "2026-10-25T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	body := jsonBody(t, map[string]any{"name": "shop-prod", "project": "shop"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", body), nil); code != http.StatusCreated {
		t.Fatalf("create stack status = %d", code)
	}
	return cookie
}

func stackApply(t *testing.T, rt *Router, cookie *http.Cookie, docs string, extra map[string]any) (int, stackApplyResponse) {
	t.Helper()
	var out stackApplyResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, docs, extra)), &out)
	return code, out
}

func TestStackRevisionsIncrementAndRollbackCreatesNewRevision(t *testing.T) {
	rt, db, cookie := stackFixture(t)

	for i, image := range []string{"ghcr.io/acme/web:1.2.3", "ghcr.io/acme/web:1.2.4"} {
		code, out := stackApply(t, rt, cookie, stackTestDocs(image), nil)
		if code != http.StatusOK || out.Revision == nil || out.Revision.Number != i+1 || out.Revision.Status != "applied" {
			t.Fatalf("apply %d: status %d %+v", i+1, code, out)
		}
	}
	svc, err := db.GetDesiredService(t.Context(), "web")
	if err != nil || svc.Image != "ghcr.io/acme/web:1.2.4" {
		t.Fatalf("app = %+v, %v", svc, err)
	}

	var rolled stackApplyResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", ""), &rolled)
	if code != http.StatusOK || rolled.Revision == nil || rolled.Revision.Number != 3 || rolled.Revision.RollbackOf != 1 {
		t.Fatalf("rollback: status %d %+v", code, rolled)
	}
	if len(rolled.Warnings) == 0 || !strings.Contains(rolled.Warnings[0], "not rolled back") {
		t.Fatalf("rollback warning missing: %+v", rolled.Warnings)
	}
	if svc, _ = db.GetDesiredService(t.Context(), "web"); svc.Image != "ghcr.io/acme/web:1.2.3" {
		t.Fatalf("rollback did not restore the image: %q", svc.Image)
	}

	var hist struct {
		Revisions []stackRevisionSummary `json:"revisions"`
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions", ""), &hist); code != http.StatusOK || len(hist.Revisions) != 3 {
		t.Fatalf("history: %d %+v", code, hist)
	}
	if hist.Revisions[0].Number != 3 || hist.Revisions[1].Status != "superseded" {
		t.Fatalf("history order/status: %+v", hist.Revisions)
	}

	var detail stackRevisionDetail
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions/2", ""), &detail)
	if len(detail.Files) != 1 || !strings.Contains(detail.Files[0].Content, "1.2.4") || len(detail.Plan) == 0 || len(detail.Result) == 0 {
		t.Fatalf("revision detail incomplete: %+v", detail)
	}

	var diff stackDiffResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions/2/diff?against=1", ""), &diff); code != http.StatusOK {
		t.Fatalf("diff status = %d", code)
	}
	if len(diff.Changes) != 1 || diff.Changes[0].Name != "web" || diff.Changes[0].Action != "changed" {
		t.Fatalf("diff = %+v", diff)
	}
	var live stackDiffResponse
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions/2/diff", ""), &live)
	if live.Against != "live" || len(live.Changes) == 0 {
		t.Fatalf("live diff should show the rollback as pending: %+v", live)
	}

	var got stackResource
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod", ""), &got)
	keys := map[string]bool{}
	for _, e := range got.Resources {
		keys[e.Key] = true
	}
	// Every declared kind is ledgered, not only apps.
	if got.LatestRevision != 3 || len(got.Resources) != 3 || !keys["App/web"] || !keys["Project/shop"] || !keys["Tag/frontend"] {
		t.Fatalf("stack = %+v", got)
	}
}

func TestStackPinBlocksApplyAndOtherRollbacks(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.1.0"), nil)

	pin := jsonBody(t, map[string]int{"revision": 1})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/stacks/shop-prod/pin", pin), nil); code != http.StatusOK {
		t.Fatalf("pin status = %d", code)
	}
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:2.0.0"), nil); code != http.StatusConflict {
		t.Fatalf("apply while pinned = %d, want 409", code)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/2/rollback", ""), nil); code != http.StatusConflict {
		t.Fatalf("rollback to another revision while pinned = %d, want 409", code)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", ""), nil); code != http.StatusOK {
		t.Fatalf("rollback to the pinned revision = %d, want 200", code)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/shop-prod/pin", ""), nil); code != http.StatusOK {
		t.Fatalf("unpin status = %d", code)
	}
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:2.0.0"), nil); code != http.StatusOK {
		t.Fatalf("apply after unpin = %d", code)
	}
}

func TestStackLeaseContentionExactlyOneApplyWins(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	const callers = 4
	entered := make(chan struct{}, callers)
	release := make(chan struct{})
	var once sync.Once
	stackAfterLease = func() {
		entered <- struct{}{}
		<-release
	}
	t.Cleanup(func() { stackAfterLease = nil })

	codes := make(chan int, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, stackTestDocs("ghcr.io/acme/web:3.0.0"), nil))
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			codes <- rec.Code
		}()
	}
	<-entered
	// The losers answer 409 while the winner still holds the lease.
	for i := 0; i < callers-1; i++ {
		if c := <-codes; c != http.StatusConflict {
			t.Fatalf("contender status = %d, want 409", c)
		}
	}
	once.Do(func() { close(release) })
	wg.Wait()
	if c := <-codes; c != http.StatusOK {
		t.Fatalf("winner status = %d, want 200", c)
	}
}

func TestStackUnlockClearsStaleLease(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	s, err := db.GetStackByName(t.Context(), "shop-prod")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AcquireStackLock(t.Context(), s.ID, "crashed-runner", 1, stackLockTTL(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusConflict {
		t.Fatalf("apply under a foreign lease = %d, want 409", code)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/unlock", ""), nil); code != http.StatusOK {
		t.Fatalf("unlock status = %d", code)
	}
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusOK {
		t.Fatalf("apply after unlock = %d", code)
	}
}

func TestStackIdempotencyKeyReplaysSameRevision(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	extra := map[string]any{"idempotency_key": "ci-run-42", "git_sha": "abc123", "git_repo": "acme/infra", "ci_run_url": "https://ci.example.com/42"}
	_, first := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), extra)
	code, second := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), extra)
	if code != http.StatusOK || !second.Replayed || second.Revision.Number != first.Revision.Number {
		t.Fatalf("replay = %d %+v (first %+v)", code, second, first.Revision)
	}
	if first.Revision.GitSHA != "abc123" || first.Revision.GitRepo != "acme/infra" || first.Revision.CIRunURL == "" {
		t.Fatalf("git metadata missing: %+v", first.Revision)
	}
}

func TestStackNeverPersistsSecretValues(t *testing.T) {
	rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
	cookie := stackSeed(t, rt, db)
	const secret = "s3cr3t-value-0xDEADBEEF" //nolint:gosec // fixture, not a credential
	docs := strings.Replace(iacTestDocs, "      LOG_LEVEL: info\n", "      LOG_LEVEL: info\n      DB_PASSWORD: {secretRef: DB_PASSWORD}\n", 1)
	code, out := stackApply(t, rt, cookie, docs, map[string]any{"secrets": map[string]string{"web/DB_PASSWORD": secret}})
	if code != http.StatusOK || out.Revision == nil || out.Revision.Status != "applied" {
		t.Fatalf("apply = %d %+v %+v %+v", code, out.Revision, out.Issues, out.Result)
	}

	s, _ := db.GetStackByName(t.Context(), "shop-prod")
	revs, err := db.ListStackRevisions(t.Context(), s.ID, 0)
	if err != nil || len(revs) == 0 {
		t.Fatalf("revisions: %v %d", err, len(revs))
	}
	for _, r := range revs {
		full, err := db.GetStackRevision(t.Context(), s.ID, r.Number)
		if err != nil {
			t.Fatal(err)
		}
		for name, blob := range map[string][]byte{"plan": full.PlanJSON, "docs": full.RenderedDocs, "result": full.ResultJSON} {
			if strings.Contains(string(blob), secret) {
				t.Fatalf("revision %d %s holds the secret value", r.Number, name)
			}
		}
	}
	rows, err := db.QueryContext(t.Context(), `SELECT COALESCE(detail, '') FROM audit_log WHERE action LIKE 'stack.%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	seen := 0
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		seen++
		if strings.Contains(d, secret) {
			t.Fatalf("audit detail holds the secret value: %s", d)
		}
	}
	if seen == 0 {
		t.Fatal("no stack audit rows written")
	}

	// A document that embeds the value is refused before it is stored.
	leaky := strings.Replace(iacTestDocs, "LOG_LEVEL: info", "LOG_LEVEL: "+secret, 1)
	code, _ = stackApply(t, rt, cookie, leaky, map[string]any{"secrets": map[string]string{"web/DB_PASSWORD": secret}})
	if code == http.StatusOK {
		t.Fatalf("a document holding a secret value was accepted")
	}
	after, _ := db.ListStackRevisions(t.Context(), s.ID, 0)
	if len(after) != len(revs) {
		t.Fatalf("a leaking apply stored a revision")
	}
}

func TestStackReadTokenCanPlanAndReadButNotWrite(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	tok := seedMatrixToken(t, db, "stack-ro", []string{AbilityRead})
	call := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		return iacCall(t, rt, req, nil)
	}
	docs := stackBody(t, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/v1/stacks/shop-prod/plan", docs, http.StatusOK},
		{http.MethodGet, "/api/v1/stacks", "", http.StatusOK},
		{http.MethodGet, "/api/v1/stacks/shop-prod/revisions", "", http.StatusOK},
		{http.MethodGet, "/api/v1/stacks/shop-prod/revisions/1/diff?against=1", "", http.StatusOK},
		{http.MethodPost, "/api/v1/stacks/shop-prod/apply", docs, http.StatusForbidden},
		{http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", "", http.StatusForbidden},
		{http.MethodPut, "/api/v1/stacks/shop-prod/pin", `{"revision":1}`, http.StatusForbidden},
		{http.MethodDelete, "/api/v1/stacks/shop-prod", "", http.StatusForbidden},
		{http.MethodPost, "/api/v1/stacks/shop-prod/unlock", "", http.StatusForbidden},
	} {
		if got := call(c.method, c.path, c.body); got != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, got, c.want)
		}
	}
}

func TestStackDeleteForgetKeepsAppsAndDestroyRefusesCI(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)

	ci := seedCIToken(t, db, "ci:acme/infra@refs/heads/main", []string{AbilityRoot})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/stacks/shop-prod?mode=destroy", nil)
	req.Header.Set("Authorization", "Bearer "+ci)
	if code := iacCall(t, rt, req, nil); code != http.StatusForbidden {
		t.Fatalf("destroy from a CI token = %d, want 403", code)
	}

	var out stackDeleteResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/shop-prod", ""), &out); code != http.StatusOK || len(out.Forgotten) != 3 {
		t.Fatalf("forget = %d %+v", code, out)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err != nil {
		t.Fatalf("forget removed the app: %v", err)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod", ""), nil); code != http.StatusNotFound {
		t.Fatalf("deleted stack still readable: %d", code)
	}
}

func TestStackDestroyRemovesLedgerApps(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	var out stackDeleteResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/stacks/shop-prod?mode=destroy", ""), &out); code != http.StatusOK || len(out.Destroyed) != 1 {
		t.Fatalf("destroy = %d %+v", code, out)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("destroy left the app behind")
	}
}

func TestStackPlanForMissingStackAndPruneGate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	var plan stackPlanResponse
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/new-stack/plan", stackBody(t, iacTestDocs, map[string]any{"project": "shop"})), &plan)
	if code != http.StatusOK || plan.StackExists || plan.Plan == nil || plan.Plan.Summary.Create != 3 {
		t.Fatalf("plan = %d %+v", code, plan)
	}

	rt, _, cookie = stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), map[string]any{"prune": true}); code != http.StatusUnprocessableEntity {
		t.Fatalf("prune without allow_prune = %d, want 422", code)
	}
	set := jsonBody(t, map[string]any{"allow_prune": true})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", set), nil); code != http.StatusOK {
		t.Fatalf("settings status = %d", code)
	}
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), map[string]any{"prune": true}); code != http.StatusOK {
		t.Fatalf("prune with allow_prune = %d, want 200", code)
	}
}

func TestStackErrorMapping(t *testing.T) {
	if !strings.Contains(mapStackStoreError(store.ErrStackLocked).Error(), iac.ErrStackLocked.Error()) {
		t.Fatal("ErrStackLocked not mapped")
	}
	if err := mapStackStoreError(store.ErrStackLedgerConflict); !errors.Is(err, iac.ErrLedgerConflict) {
		t.Fatalf("ErrStackLedgerConflict mapped to %v", err)
	}
	if err := mapStackStoreError(store.ErrStackLocked); !errors.Is(err, iac.ErrStackLocked) {
		t.Fatalf("ErrStackLocked mapped to %v", err)
	}
}

func TestStackRevisionTransitionParity(t *testing.T) {
	all := []iac.RevisionStatus{iac.RevisionPendingApproval, iac.RevisionApplying, iac.RevisionApplied, iac.RevisionPartial, iac.RevisionFailed, iac.RevisionSuperseded}
	for _, from := range all {
		for _, to := range all {
			if got, want := store.StackRevisionCanTransition(string(from), string(to)), from.CanTransition(to); got != want {
				t.Errorf("%s -> %s: store %v, iac %v", from, to, got, want)
			}
		}
	}
}

func TestStackRevisionJSONShape(t *testing.T) {
	raw, err := json.Marshal(stackRevisionSummary{Number: 2, Status: "applied", RollbackOf: 1})
	if err != nil || !strings.Contains(string(raw), `"rollback_of":1`) {
		t.Fatalf("summary json = %s %v", raw, err)
	}
}

func seedCIToken(t *testing.T, db *store.DB, name string, abilities []string) string {
	t.Helper()
	plain := "ci-secret-" + name
	if err := db.SaveAPIToken(t.Context(), store.APIToken{ID: "ci-token-1", Name: name, TokenHash: hashToken(plain), Abilities: abilities}); err != nil {
		t.Fatalf("seed ci token: %v", err)
	}
	return plain
}

func TestSummarizeRevisionMarksRejected(t *testing.T) {
	rej := summarizeRevision(store.StackRevisionRecord{Number: 3, Status: store.StackRevFailed, ResultJSON: []byte(`{"error":"rejected","rejected_by":"qa2"}`)})
	if !rej.Rejected || rej.Status != store.StackRevFailed {
		t.Fatalf("rejected summary = %+v", rej)
	}
	for _, v := range []store.StackRevisionRecord{
		{Status: store.StackRevFailed, ResultJSON: []byte(`{"error":"boom"}`)},
		{Status: store.StackRevFailed},
		{Status: store.StackRevApplied, ResultJSON: []byte(`{"error":"rejected"}`)},
	} {
		if summarizeRevision(v).Rejected {
			t.Errorf("wrongly marked rejected: %+v", v)
		}
	}
}
