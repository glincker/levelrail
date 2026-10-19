package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mintJoinTokenViaAPI(t *testing.T, rt *Router, cookie *http.Cookie) createNodeJoinTokenResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got createNodeJoinTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func listPendingJoinTokens(t *testing.T, rt *Router, cookie *http.Cookie) []pendingJoinTokenResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/join-tokens", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []pendingJoinTokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestJoinTokens_ListAndRevokeOne(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	keep := mintJoinTokenViaAPI(t, rt, cookie)
	drop := mintJoinTokenViaAPI(t, rt, cookie)

	pending := listPendingJoinTokens(t, rt, cookie)
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	dropRow, err := db.GetNodeJoinTokenByHash(context.Background(), hashToken(drop.Token))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/join-tokens/"+dropRow.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := listPendingJoinTokens(t, rt, cookie); len(got) != 1 {
		t.Fatalf("pending after revoke = %d, want 1", len(got))
	}
	revoked, _ := db.GetNodeJoinTokenByHash(context.Background(), hashToken(drop.Token))
	if revoked.ExpiresAt.After(time.Now()) {
		t.Errorf("revoked token still expires in the future: %v", revoked.ExpiresAt)
	}
	kept, _ := db.GetNodeJoinTokenByHash(context.Background(), hashToken(keep.Token))
	if !kept.ExpiresAt.After(time.Now()) {
		t.Errorf("untouched token was expired: %v", kept.ExpiresAt)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/join-tokens/"+dropRow.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("second revoke status = %d, want 404", rec.Code)
	}
}

func TestJoinTokens_RevokeAll(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	mintJoinTokenViaAPI(t, rt, cookie)
	mintJoinTokenViaAPI(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/join-tokens", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got revokeJoinTokensResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Revoked != 2 {
		t.Fatalf("revoked = %+v, err = %v, want 2", got, err)
	}
	if left := listPendingJoinTokens(t, rt, cookie); len(left) != 0 {
		t.Errorf("pending after revoke all = %d, want 0", len(left))
	}
}

func TestJoinTokens_AdminRoutesRequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/nodes/join-tokens"},
		{http.MethodDelete, "/api/v1/nodes/join-tokens"},
		{http.MethodDelete, "/api/v1/nodes/join-tokens/njt_1"},
	})
}
