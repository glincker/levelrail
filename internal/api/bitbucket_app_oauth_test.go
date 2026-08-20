package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedBitbucketAppConnection(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveBitbucketAppConnection(context.Background(), store.BitbucketAppConnection{
		Key: "the-key", CreatedAt: "2026-08-20T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed bitbucket app connection: %v", err)
	}
}

func TestHandleStartBitbucketAppConnect_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBitbucketAppSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/connect", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartBitbucketAppConnect_NotConnected(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/connect", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartBitbucketAppConnect_NoPrimaryDomain(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/connect", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartBitbucketAppConnect_Success(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)
	setPrimaryDomain(t, db)

	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/connect", "")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body = %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://bitbucket.org/site/oauth2/authorize?") {
		t.Errorf("Location = %q, want it to start with Bitbucket's own authorize endpoint", loc)
	}
	if !strings.Contains(loc, "client_id=the-key") {
		t.Errorf("Location = %q, missing client_id", loc)
	}
	if !strings.Contains(loc, "redirect_uri=https%3A%2F%2Fdeploy.example.com%2Fapi%2Fv1%2Fbitbucket-app%2Fcallback") {
		t.Errorf("Location = %q, redirect_uri does not match this control plane's own callback path", loc)
	}
}

func TestHandleBitbucketAppCallback_MissingCode(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/callback?state=x", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleBitbucketAppCallback_InvalidState(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/callback?code=abc&state=never-issued", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleBitbucketAppCallback_Success drives the real pendingState:
// it starts a connect first (to get a real, valid state value), then
// completes the callback with it, and asserts the exchanged tokens
// landed in secrets, never in the redirect itself.
func TestHandleBitbucketAppCallback_Success(t *testing.T) {
	fakeClient := &fakeBitbucketAppClient{
		exchangeTokens: bitbucketapp.Tokens{AccessToken: "at-1", RefreshToken: "rt-1", ExpiresAt: time.Now().Add(2 * time.Hour)},
	}
	secrets := newFakeBitbucketAppSecrets()
	rt, db := newTestRouterWithBitbucketApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)
	setPrimaryDomain(t, db)
	if err := secrets.SetValue(context.Background(), store.BitbucketAppSecretsKey(), bitbucketAppSecretKey, "the-secret"); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	startRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(startRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/connect", ""))
	if startRec.Code != http.StatusFound {
		t.Fatalf("start status = %d, body = %s", startRec.Code, startRec.Body.String())
	}
	loc, err := url.Parse(startRec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatal("Location has no state param")
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/callback?code=the-code&state="+state, ""))
	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302, body = %s", rec.Code, rec.Body.String())
	}
	if fakeClient.gotCode != "the-code" {
		t.Errorf("client got code = %q, want the-code", fakeClient.gotCode)
	}
	if strings.Contains(rec.Header().Get("Location"), "at-1") {
		t.Error("redirect Location leaks the access token")
	}

	statusRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(statusRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app", ""))
	if !strings.Contains(statusRec.Body.String(), `"authorized":true`) {
		t.Errorf("status after callback = %s, want authorized:true", statusRec.Body.String())
	}
}

func TestBitbucketAccessToken_RefreshesNearExpiry(t *testing.T) {
	fakeClient := &fakeBitbucketAppClient{
		refreshTokens: bitbucketapp.Tokens{AccessToken: "at-fresh", RefreshToken: "rt-fresh", ExpiresAt: time.Now().Add(2 * time.Hour)},
	}
	secrets := newFakeBitbucketAppSecrets()
	rt, db := newTestRouterWithBitbucketApp(t, secrets, fakeClient)
	ctx := context.Background()
	seedBitbucketAppConnection(t, db)

	key := store.BitbucketAppSecretsKey()
	mustSetBitbucketSecret(t, secrets, key, bitbucketAppSecretKey, "the-secret")
	mustSetBitbucketSecret(t, secrets, key, bitbucketAppAccessTokenKey, "at-stale")
	mustSetBitbucketSecret(t, secrets, key, bitbucketAppRefreshTokenKey, "rt-stale")
	// Already past expiry, forcing the refresh path.
	mustSetBitbucketSecret(t, secrets, key, bitbucketAppTokenExpiresKey, "1")

	token, err := rt.bitbucketAccessToken(ctx)
	if err != nil {
		t.Fatalf("bitbucketAccessToken() error = %v", err)
	}
	if !fakeClient.refreshCalled {
		t.Error("RefreshToken was not called for a near-expiry token")
	}
	if token != "at-fresh" {
		t.Errorf("token = %q, want the freshly refreshed at-fresh", token)
	}
}

func TestBitbucketAccessToken_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	seedBitbucketAppConnection(t, db)

	_, err := rt.bitbucketAccessToken(context.Background())
	if err == nil {
		t.Fatal("bitbucketAccessToken() error = nil, want errBitbucketAppNotAuthorized")
	}
}

func mustSetBitbucketSecret(t *testing.T, s *fakeBitbucketAppSecrets, key, envKey, val string) {
	t.Helper()
	if err := s.SetValue(context.Background(), key, envKey, val); err != nil {
		t.Fatalf("seed %s: %v", envKey, err)
	}
}
