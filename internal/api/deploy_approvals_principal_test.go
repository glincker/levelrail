package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedApprovalToken(t *testing.T, db *store.DB, id, owner string) string {
	t.Helper()
	plain := "approval-secret-" + id
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: id, Name: id, TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityDeploy},
		CreatedAt: time.Now(), OwnerUserID: owner,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return plain
}

func bearerApprove(rt *Router, plain, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func bearerDeploy(t *testing.T, rt *Router, plain string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/deploys", strings.NewReader(`{"image":"`+requestPendingApprovalImage+`","confirm":true}`))
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("token request approval: status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var result deployTriggerResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.PendingApproval == nil {
		t.Fatalf("expected a pending approval, err = %v, body = %s", err, rec.Body.String())
	}
	return result.PendingApproval.ID
}

// A user must not approve their own session request with their own token.
func TestDeployApproval_SameUserViaTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, u.ID))
	plain := seedApprovalToken(t, db, "tok_self", u.ID)

	if rec := bearerApprove(rt, plain, id); rec.Code != http.StatusForbidden {
		t.Fatalf("self-approve via own token: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusPending {
		t.Errorf("Status = %q, want pending", a.Status)
	}
}

// The reverse direction: a token request approved by the token owner's session.
func TestDeployApproval_TokenRequestOwnerSessionForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	plain := seedApprovalToken(t, db, "tok_req", u.ID)
	id := bearerDeploy(t, rt, plain)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, sessionCookieForTest(t, rt, u.ID), http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("owner session approving own token request: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

func TestDeployApproval_DifferentUserViaTokenAllowed(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	v := storeUserWithAbilitiesForTest(t, db, "v@example.com", []string{AbilityRead, AbilityDeploy})
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, u.ID))
	plain := seedApprovalToken(t, db, "tok_other", v.ID)

	if rec := bearerApprove(rt, plain, id); rec.Code != http.StatusOK {
		t.Fatalf("different user via token: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// A system token with no owner keeps the (type, id) comparison.
func TestDeployApproval_UnownedTokenBehavesAsBefore(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	ci := seedApprovalToken(t, db, "tok_ci", "")

	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, u.ID))
	if rec := bearerApprove(rt, ci, id); rec.Code != http.StatusOK {
		t.Fatalf("unowned token approving a user request: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("reset app: %v", err)
	}
	selfID := bearerDeploy(t, rt, ci)
	if rec := bearerApprove(rt, ci, selfID); rec.Code != http.StatusForbidden {
		t.Fatalf("unowned token approving its own request: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}
