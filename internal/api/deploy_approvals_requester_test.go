package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// failingTokenLookup injects a store failure into the by-id token lookup.
type failingTokenLookup struct{ TokenStore }

func (failingTokenLookup) GetAPITokenByID(context.Context, string) (*store.APIToken, error) {
	return nil, errors.New("injected token store failure")
}

func sessionApprove(t *testing.T, rt *Router, userID, id string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, sessionCookieForTest(t, rt, userID), http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	return rec
}

func assertTokenGone(t *testing.T, db *store.DB, id string) {
	t.Helper()
	if _, err := db.GetAPITokenByID(context.Background(), id); !errors.Is(err, store.ErrAPITokenNotFound) {
		t.Fatalf("token %s still present after purge: err = %v", id, err)
	}
}

func purgeTokensForTest(t *testing.T, db *store.DB, kind store.TokenPurgeKind) {
	t.Helper()
	n, err := db.PurgeInactiveAPITokens(context.Background(), kind, time.Now().Add(48*time.Hour), 100, false)
	if err != nil || n == 0 {
		t.Fatalf("purge %s tokens: n = %d, err = %v", kind, n, err)
	}
}

// legacyTokenRequest rewrites a pending row as if a token had requested it
// before the requester column existed.
func legacyTokenRequest(t *testing.T, db *store.DB, id, tokenID string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`UPDATE deploy_approvals SET requested_by_type = ?, requested_by = ?, requester_user_id = NULL WHERE id = ?`,
		store.PrincipalTypeToken, tokenID, id); err != nil {
		t.Fatalf("rewrite approval as legacy: %v", err)
	}
}

// A root user requests through their own token, revokes and hard deletes it,
// then approves from their session: still their own request.
func TestDeployApproval_PurgedRevokedTokenOwnerSessionForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "root@example.com", []string{AbilityRoot, AbilityRead, AbilityDeploy})
	plain := seedApprovalToken(t, db, "tok_purged", u.ID)
	id := bearerDeploy(t, rt, plain)
	cookie := sessionCookieForTest(t, rt, u.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/auth/tokens/tok_purged", ""))
	if rec.Code >= 300 {
		t.Fatalf("revoke token: status = %d; body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens/cleanup", `{"older_than_days":0,"confirm":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup tokens: status = %d; body = %s", rec.Code, rec.Body.String())
	}
	assertTokenGone(t, db, "tok_purged")

	if rec := sessionApprove(t, rt, u.ID, id); rec.Code != http.StatusForbidden {
		t.Fatalf("owner session after token purge: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

// The scheduled expiry purge can remove the token while the approval is still pending.
func TestDeployApproval_PurgedExpiredTokenOwnerSessionForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	soon := time.Now().Add(time.Hour)
	plain := "approval-secret-tok_exp"
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_exp", Name: "tok_exp", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityDeploy},
		CreatedAt: time.Now(), ExpiresAt: &soon, OwnerUserID: u.ID,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	id := bearerDeploy(t, rt, plain)
	purgeTokensForTest(t, db, store.TokenPurgeExpired)
	assertTokenGone(t, db, "tok_exp")

	if rec := sessionApprove(t, rt, u.ID, id); rec.Code != http.StatusForbidden {
		t.Fatalf("owner session after expiry purge: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

// A legacy row whose owned token lookup errors must not fall back to the token pair.
func TestDeployApproval_LegacyRowTokenStoreErrorDenies(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	other := storeUserWithAbilitiesForTest(t, db, "o@example.com", []string{AbilityRead, AbilityDeploy})
	seedApprovalToken(t, db, "tok_mirror", u.ID)
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, other.ID))
	legacyTokenRequest(t, db, id, "tok_mirror")
	rt.tokens = failingTokenLookup{rt.tokens}

	if rec := sessionApprove(t, rt, u.ID, id); rec.Code == http.StatusOK {
		t.Fatalf("token store error: status = 200, want a refusal; body = %s", rec.Body.String())
	}
}

func TestDeployApproval_LegacyRowOwnedTokenStillResolved(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	other := storeUserWithAbilitiesForTest(t, db, "o@example.com", []string{AbilityRead, AbilityDeploy})
	seedApprovalToken(t, db, "tok_mirror", u.ID)
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, other.ID))
	legacyTokenRequest(t, db, id, "tok_mirror")

	if rec := sessionApprove(t, rt, u.ID, id); rec.Code != http.StatusForbidden {
		t.Fatalf("legacy owned token, owner session: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

// A legacy row whose token is gone cannot prove who stood behind it.
func TestDeployApproval_LegacyRowMissingTokenDenies(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	other := storeUserWithAbilitiesForTest(t, db, "o@example.com", []string{AbilityRead, AbilityDeploy})
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, other.ID))
	legacyTokenRequest(t, db, id, "tok_vanished")

	if rec := sessionApprove(t, rt, u.ID, id); rec.Code != http.StatusForbidden {
		t.Fatalf("legacy row with a missing token: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

// The decider's own token lookup failing must refuse, not compare token pairs.
func TestDeployApproval_DeciderTokenStoreErrorDenies(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, u.ID))
	plain := seedApprovalToken(t, db, "tok_self", u.ID)
	rt.tokens = failingTokenLookup{rt.tokens}

	if rec := bearerApprove(rt, plain, id); rec.Code == http.StatusOK {
		t.Fatalf("decider token store error: status = 200, want a refusal; body = %s", rec.Body.String())
	}
}

func TestDeployApproval_StoresRequesterOwner(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	id := bearerDeploy(t, rt, seedApprovalToken(t, db, "tok_req", u.ID))
	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if !a.RequesterRecorded || a.RequesterUserID != u.ID {
		t.Fatalf("requester = (%q, recorded %v), want (%q, true)", a.RequesterUserID, a.RequesterRecorded, u.ID)
	}
}

// Two-person flow: an ownerless CI token requests, the token is later
// purged, and a human approves.
func TestDeployApproval_OwnerlessCITokenPurgedHumanApproves(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProtectedDeployFixture(t, db)
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	id := bearerDeploy(t, rt, seedApprovalToken(t, db, "tok_ci", ""))
	if err := db.RevokeAPIToken(context.Background(), "tok_ci"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	purgeTokensForTest(t, db, store.TokenPurgeRevoked)
	assertTokenGone(t, db, "tok_ci")

	if rec := sessionApprove(t, rt, u.ID, id); rec.Code != http.StatusOK {
		t.Fatalf("human approving a purged CI token request: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

func TestTokenOwnerOf_FailsClosed(t *testing.T) {
	rt, db := newTestRouter(t)
	if _, err := rt.tokenOwnerOf(context.Background(), "tok_none"); !errors.Is(err, errApprovalOwnerUnknown) {
		t.Fatalf("missing token: err = %v, want errApprovalOwnerUnknown", err)
	}
	seedApprovalToken(t, db, "tok_ci", "")
	if owner, err := rt.tokenOwnerOf(context.Background(), "tok_ci"); err != nil || owner != "" {
		t.Fatalf("ownerless token: owner = %q, err = %v", owner, err)
	}
	rt.tokens = failingTokenLookup{rt.tokens}
	if _, err := rt.tokenOwnerOf(context.Background(), "tok_ci"); err == nil {
		t.Fatal("store error: want an error, got nil")
	}
}
