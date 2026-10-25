package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// approvalFixture returns a stack that requires approval, with the admin
// session as requester and a second root token as approver.
func approvalFixture(t *testing.T) (*Router, *store.DB, *http.Cookie, string) {
	t.Helper()
	rt, db, cookie := stackFixture(t)
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/stacks/shop-prod", `{"require_approval":true}`), nil); code != http.StatusOK {
		t.Fatalf("enable approval: %d", code)
	}
	return rt, db, cookie, seedApproverToken(t, db, "approver", []string{AbilityRoot})
}

// seedApproverToken is a root-capable token owned by a second admin user, the
// shape of a real approver (ownerless tokens cannot approve).
func seedApproverToken(t *testing.T, db *store.DB, id string, abilities []string) string {
	t.Helper()
	u := storeUserWithAbilitiesForTest(t, db, id+"@example.test", []string{AbilityRoot})
	plain := "owned-secret-" + id
	if err := db.SaveAPIToken(t.Context(), store.APIToken{
		ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, OwnerUserID: u.ID,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return plain
}

func TestApprovalSelfApprovalDeniedAfterRename(t *testing.T) {
	rt, _, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPatch, "/api/v1/auth/profile", `{"display_name":"Someone Else"}`), nil); code != http.StatusOK {
		t.Fatalf("rename = %d", code)
	}
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/approve", ""), nil); code != http.StatusForbidden {
		t.Fatalf("approve after rename = %d", code)
	}
}

func TestApprovalSelfApprovalDeniedWithOwnMintedToken(t *testing.T) {
	rt, _, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	var minted createTokenResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"my-approver","abilities":["root"]}`), &minted); code != http.StatusCreated || minted.Token == "" {
		t.Fatalf("mint = %d", code)
	}
	if code := tokenPost(t, rt, minted.Token, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusForbidden {
		t.Fatalf("approve with own token = %d", code)
	}
}

func TestApprovalOwnerlessApproverDenied(t *testing.T) {
	rt, db, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	tok := seedMatrixToken(t, db, "ownerless", []string{AbilityRoot})
	if code := tokenPost(t, rt, tok, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusForbidden {
		t.Fatalf("ownerless approve = %d", code)
	}
}

func tokenPost(t *testing.T, rt *Router, tok, path, body string, out any) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	return iacCall(t, rt, req, out)
}

func TestApprovalApplyStoresPendingAndAppliesNothing(t *testing.T) {
	rt, db, cookie, _ := approvalFixture(t)
	code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), map[string]any{"secrets": map[string]string{"TOKEN": "hunter2-secret-value"}})
	if code != http.StatusAccepted || !out.PendingApproval || out.Revision == nil || out.Revision.Status != "pending_approval" || out.Revision.Number != 1 || out.ExpiresAt == "" {
		t.Fatalf("apply = %d %+v", code, out)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("pending apply must not create the app")
	}
	rev, err := db.GetStackRevision(t.Context(), mustStackID(t, db), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, blob := range [][]byte{rev.RenderedDocs, rev.PlanJSON, rev.ResultJSON} {
		if strings.Contains(string(blob), "hunter2-secret-value") {
			t.Fatalf("secret stored: %s", blob)
		}
	}
}

func mustStackID(t *testing.T, db *store.DB) string {
	t.Helper()
	s, err := db.GetStackByName(t.Context(), "shop-prod")
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func TestApprovalSelfApprovalDenied(t *testing.T) {
	rt, db, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/approve", ""), nil); code != http.StatusForbidden {
		t.Fatalf("self approve = %d", code)
	}
	// The same token that requested it is denied too.
	tok := seedMatrixToken(t, db, "requester", []string{AbilityRoot})
	body := stackBody(t, stackTestDocs("ghcr.io/acme/web:1.0.1"), nil)
	if code := tokenPost(t, rt, tok, "/api/v1/stacks/shop-prod/apply", body, nil); code != http.StatusAccepted {
		t.Fatalf("token apply = %d", code)
	}
	if code := tokenPost(t, rt, tok, "/api/v1/stacks/shop-prod/revisions/2/approve", "", nil); code != http.StatusForbidden {
		t.Fatalf("same token approve = %d", code)
	}
}

func TestApprovalApproveAppliesAndRecordsRevision(t *testing.T) {
	rt, db, cookie, approver := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	var out stackApplyResponse
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", &out); code != http.StatusOK {
		t.Fatalf("approve = %d %+v", code, out)
	}
	if out.Revision == nil || out.Revision.Number != 1 || out.Revision.Status != "applied" || out.Result == nil {
		t.Fatalf("approve response = %+v", out)
	}
	if svc, err := db.GetDesiredService(t.Context(), "web"); err != nil || svc.Image != "ghcr.io/acme/web:1.0.0" {
		t.Fatalf("app = %+v %v", svc, err)
	}
	// No extra revision was created, and a second approve is refused.
	var hist struct {
		Revisions []stackRevisionSummary `json:"revisions"`
	}
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions", ""), &hist)
	if len(hist.Revisions) != 1 || hist.Revisions[0].Status != "applied" {
		t.Fatalf("history = %+v", hist.Revisions)
	}
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusConflict {
		t.Fatalf("second approve = %d", code)
	}
}

func TestApprovalExpiredRefusedAndMarkedFailedOnRead(t *testing.T) {
	rt, _, cookie, approver := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	t.Setenv("APP_STACK_APPROVAL_TTL", "1ns")
	time.Sleep(2 * time.Millisecond)
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusGone {
		t.Fatalf("expired approve = %d", code)
	}
	var hist struct {
		Revisions []stackRevisionSummary `json:"revisions"`
	}
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions", ""), &hist)
	if len(hist.Revisions) != 1 || hist.Revisions[0].Status != "failed" {
		t.Fatalf("expired revision = %+v", hist.Revisions)
	}
}

func TestApprovalExpiredMarkedFailedLazilyOnRead(t *testing.T) {
	rt, _, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	t.Setenv("APP_STACK_APPROVAL_TTL", "1ns")
	time.Sleep(2 * time.Millisecond)
	var got stackResource
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod", ""), &got)
	if got.LatestStatus != "failed" {
		t.Fatalf("latest status = %q", got.LatestStatus)
	}
}

func TestApprovalStalePlanHashIs409(t *testing.T) {
	rt, _, cookie, approver := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", `{"expected_plan_hash":"stale"}`, nil); code != http.StatusConflict {
		t.Fatalf("stale body hash = %d", code)
	}
	// Live state moves after the request: the same documents are applied outside the stack.
	direct := iacRequestBody(t, stackTestDocs("ghcr.io/acme/web:1.0.0"), map[string]any{"source": "out-of-band"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", direct), nil); code != http.StatusOK {
		t.Fatalf("direct apply = %d", code)
	}
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusConflict {
		t.Fatalf("stale plan approve = %d", code)
	}
}

func TestApprovalRejectMovesToFailedWithReason(t *testing.T) {
	rt, db, cookie, approver := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	var out stackRejectResponse
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/reject", `{"reason":"not today"}`, &out); code != http.StatusOK {
		t.Fatalf("reject = %d", code)
	}
	if out.Revision.Status != "failed" || out.Reason != "not today" {
		t.Fatalf("reject response = %+v", out)
	}
	rev, _ := db.GetStackRevision(t.Context(), mustStackID(t, db), 1)
	if rev.Status != "failed" || !strings.Contains(string(rev.ResultJSON), "not today") {
		t.Fatalf("stored = %s %s", rev.Status, rev.ResultJSON)
	}
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusConflict {
		t.Fatalf("approve after reject = %d", code)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("rejected revision must not apply")
	}
}

func TestApprovalReadOnlyAndWriteTokensDenied(t *testing.T) {
	rt, db, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	for _, ab := range []string{AbilityRead, AbilityWrite} {
		tok := seedMatrixToken(t, db, "lim-"+ab, []string{ab})
		for _, verb := range []string{"approve", "reject"} {
			if code := tokenPost(t, rt, tok, "/api/v1/stacks/shop-prod/revisions/1/"+verb, "", nil); code != http.StatusForbidden {
				t.Errorf("%s %s = %d", ab, verb, code)
			}
		}
	}
}

func TestApprovalRollbackAlsoRequiresApproval(t *testing.T) {
	rt, _, cookie, approver := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil)
	// A changed second revision so the rollback target differs from the current one (equal docs answer a no-op).
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.1.0"), nil)
	tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/2/approve", "", nil)
	var out stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/revisions/1/rollback", ""), &out); code != http.StatusAccepted || out.Revision == nil || out.Revision.RollbackOf != 1 || out.Revision.Number != 3 {
		t.Fatalf("rollback = %d %+v", code, out)
	}
}

func TestApprovalRevisionSummaryCarriesExpiry(t *testing.T) {
	t.Setenv("APP_STACK_APPROVAL_TTL", "2h")
	rt, _, cookie, _ := approvalFixture(t)
	stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil)
	var hist struct {
		Revisions []stackRevisionSummary `json:"revisions"`
	}
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions", ""), &hist)
	if len(hist.Revisions) != 1 || hist.Revisions[0].ExpiresAt == "" {
		t.Fatalf("history = %+v", hist.Revisions)
	}
	created, _ := time.Parse(time.RFC3339Nano, hist.Revisions[0].CreatedAt)
	expires, _ := time.Parse(time.RFC3339Nano, hist.Revisions[0].ExpiresAt)
	if got := expires.Sub(created); got != 2*time.Hour {
		t.Fatalf("expiry offset = %v", got)
	}
}

func TestApprovalOwnerlessRequesterTokenApprovableByAnotherUser(t *testing.T) {
	rt, db, _, approver := approvalFixture(t)
	bot := seedMatrixToken(t, db, "deploy-bot", []string{AbilityRoot})
	if code := tokenPost(t, rt, bot, "/api/v1/stacks/shop-prod/apply", stackBody(t, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil), nil); code != http.StatusAccepted {
		t.Fatalf("bot apply = %d", code)
	}
	if code := tokenPost(t, rt, bot, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusForbidden {
		t.Fatalf("bot self approve = %d", code)
	}
	if code := tokenPost(t, rt, approver, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusOK {
		t.Fatalf("approve = %d", code)
	}
}

func TestApprovalRowWithoutRequesterIDUsesTokenOwner(t *testing.T) {
	rt, db, _, _ := approvalFixture(t)
	owner := storeUserWithAbilitiesForTest(t, db, "owner@example.test", []string{AbilityRoot})
	mk := func(id string) string {
		plain := "owned-secret-" + id
		if err := db.SaveAPIToken(t.Context(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: []string{AbilityRoot}, OwnerUserID: owner.ID}); err != nil {
			t.Fatal(err)
		}
		return plain
	}
	req, appr := mk("req-tok"), mk("appr-tok")
	if code := tokenPost(t, rt, req, "/api/v1/stacks/shop-prod/apply", stackBody(t, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil), nil); code != http.StatusAccepted {
		t.Fatalf("apply = %d", code)
	}
	// Rows written before the requester column existed carry only the token id.
	if _, err := db.ExecContext(t.Context(), `UPDATE stack_revisions SET requester_user_id = ''`); err != nil {
		t.Fatal(err)
	}
	if code := tokenPost(t, rt, appr, "/api/v1/stacks/shop-prod/revisions/1/approve", "", nil); code != http.StatusForbidden {
		t.Fatalf("same owner approve = %d", code)
	}
}
