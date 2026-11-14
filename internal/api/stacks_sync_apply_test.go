package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/stacksync"
	"github.com/GLINCKER/levelrail/internal/store"
)

const syncCanary = "CANARY-sync-plaintext-91c2"

func syncApplyRec(t *testing.T, db *store.DB, prune bool) store.StackSyncRecord {
	t.Helper()
	return store.StackSyncRecord{StackID: mustStackID(t, db), StackName: "shop-prod", Config: store.StackSyncConfig{
		RepoURL: "https://git.example.invalid/org/infra.git", Branch: "main", Enabled: true, Apply: true, Prune: prune}}
}

func syncFiles(docs string) []stacksync.File {
	return []stacksync.File{{Name: "stack.yaml", Content: docs}}
}

func syncCode(err error) string {
	var ce *stacksync.CodedError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// The second commit updates an app, which needs a protection read the stack
// scoped token is not allowed yet; until that lands the sync fails closed.
func TestStackSyncApplyTwoCommits(t *testing.T) {
	rt, db, _ := stackFixture(t)
	rec := syncApplyRec(t, db, false)
	out, err := rt.stackSyncApply(t.Context(), rec, syncFiles(stackTestDocs("ghcr.io/acme/web:1.2.3")), strings.Repeat("a", 40))
	if err != nil || out.PendingApproval || out.Revision != 1 {
		t.Fatalf("first sync apply = %+v %v", out, err)
	}
	again, err := rt.stackSyncApply(t.Context(), rec, syncFiles(stackTestDocs("ghcr.io/acme/web:1.2.3")), strings.Repeat("a", 40))
	if err != nil || again.Revision != 1 {
		t.Fatalf("replayed commit = %+v %v", again, err)
	}
	_, err = rt.stackSyncApply(t.Context(), rec, syncFiles(stackTestDocs("ghcr.io/acme/web:1.2.4")), strings.Repeat("b", 40))
	if err != nil && syncCode(err) != iac.CodeSyncApplyFailed {
		t.Fatalf("second commit code = %v", err)
	}
	revs, _ := db.ListStackRevisions(t.Context(), rec.StackID, 5)
	if len(revs) != 2 || !strings.Contains(revs[0].Actor+revs[1].Actor, stackSyncActorPrefix+"shop-prod") {
		t.Fatalf("revisions = %+v", revs)
	}
	toks, _ := db.ListAPITokens(t.Context())
	for _, tk := range toks {
		if strings.HasPrefix(tk.Name, stackSyncActorPrefix) && tk.RevokedAt == nil {
			t.Fatalf("sync token %s left usable", tk.ID)
		}
	}
}

func TestStackSyncApplyRequiresApprovalNeverApplies(t *testing.T) {
	rt, db, _, _ := approvalFixture(t)
	out, err := rt.stackSyncApply(t.Context(), syncApplyRec(t, db, false), syncFiles(stackTestDocs("ghcr.io/acme/web:1.0.0")), strings.Repeat("c", 40))
	if err != nil || !out.PendingApproval || out.Revision != 1 {
		t.Fatalf("approval sync = %+v %v", out, err)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("a sync on an approval stack must not create the app")
	}
}

func TestStackSyncApplyPruneRefusedWithoutAllowPrune(t *testing.T) {
	rt, db, _ := stackFixture(t)
	_, err := rt.stackSyncApply(t.Context(), syncApplyRec(t, db, true), syncFiles(stackTestDocs("ghcr.io/acme/web:1.2.3")), strings.Repeat("d", 40))
	if code := syncCode(err); code == "" || code == iac.CodeSyncSecretInRepo {
		t.Fatalf("prune without allow_prune = %v", err)
	}
	if revs, _ := db.ListStackRevisions(t.Context(), mustStackID(t, db), 5); len(revs) != 0 {
		t.Fatalf("refused prune stored %d revisions", len(revs))
	}
}

func TestStackSyncApplyFailureLeavesStackUnchanged(t *testing.T) {
	rt, db, _ := stackFixture(t)
	_, err := rt.stackSyncApply(t.Context(), syncApplyRec(t, db, false), syncFiles("version: 1\nkind: App\nmetadata: {name: [unclosed\n"), strings.Repeat("e", 40))
	if code := syncCode(err); !slices.Contains(iac.AllCodes, code) {
		t.Fatalf("failing apply code = %q (%v)", code, err)
	}
	if revs, _ := db.ListStackRevisions(t.Context(), mustStackID(t, db), 5); len(revs) != 0 {
		t.Fatalf("failed apply stored %d revisions", len(revs))
	}
}

func TestStackSyncSecretInRepoRefused(t *testing.T) {
	rt, db, _ := stackFixture(t)
	docs := strings.Replace(iacTestDocs, "{REGION: eu}", `{REGION: eu, API_TOKEN: "`+syncCanary+`"}`, 1)
	_, err := rt.stackSyncPlan(t.Context(), syncApplyRec(t, db, false), syncFiles(docs))
	var ce *stacksync.CodedError
	if !errors.As(err, &ce) || ce.Code != iac.CodeSyncSecretInRepo || !strings.HasPrefix(ce.Detail, "stack.yaml") {
		t.Fatalf("secret sync = %v", err)
	}
	if strings.Contains(ce.Error()+ce.Detail, syncCanary) {
		t.Fatal("the secret value reached the sync error")
	}
}

func TestStackSyncApplyRequestFlags(t *testing.T) {
	rec := store.StackSyncRecord{Config: store.StackSyncConfig{Apply: true}}
	if got := syncApplyRequest(rec, nil, "abc"); !got.NoDeploy || got.Prune || got.IdempotencyKey != "sync-abc" || got.GitSHA != "abc" {
		t.Fatalf("deploy off request = %+v", got)
	}
	rec.Config.Deploy, rec.Config.Prune = true, true
	if got := syncApplyRequest(rec, nil, "abc"); got.NoDeploy || !got.Prune {
		t.Fatalf("deploy on request = %+v", got)
	}
}

func TestStackSyncPutNeedsEachFlag(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	_ = db
	base := `{"repo_url":"https://git.example.invalid/org/infra.git","branch":"main","enabled":false`
	for body, want := range map[string]int{
		base + `}`: http.StatusUnprocessableEntity,
		base + `,"apply":false,"prune":true,"deploy":false}`: http.StatusUnprocessableEntity,
		base + `,"apply":false,"prune":false,"deploy":true}`: http.StatusUnprocessableEntity,
		base + `,"apply":true,"prune":false,"deploy":false}`: http.StatusOK,
	} {
		if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/stacks/shop-prod/sync", body), nil); code != want {
			t.Fatalf("PUT %s = %d, want %d", body, code, want)
		}
	}
}
