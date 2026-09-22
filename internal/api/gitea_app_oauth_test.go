package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedGiteaAppConnection(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveGiteaAppConnection(context.Background(), store.GiteaAppConnection{
		InstanceURL: "https://git.example.com", ClientID: "cid", CreatedAt: "2026-08-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed gitea app connection: %v", err)
	}
}

func TestHandleStartGiteaAppConnect_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGiteaAppSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/connect", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGiteaAppConnect_NotConnected(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/connect", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGiteaAppConnect_NoPrimaryDomain(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/connect", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGiteaAppConnect_Success(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)
	setPrimaryDomain(t, db)

	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/connect", "")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body = %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://git.example.com/login/oauth/authorize?") {
		t.Errorf("Location = %q, want it to start with the instance's own authorize endpoint", loc)
	}
	if !strings.Contains(loc, "client_id=cid") {
		t.Errorf("Location = %q, missing client_id", loc)
	}
	if !strings.Contains(loc, "redirect_uri=https%3A%2F%2Fdeploy.example.com%2Fapi%2Fv1%2Fgitea-app%2Fcallback") {
		t.Errorf("Location = %q, redirect_uri does not match this control plane's own callback path", loc)
	}
}

func TestHandleGiteaAppCallback_MissingCode(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/callback?state=x", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGiteaAppCallback_InvalidState(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/callback?code=abc&state=never-issued", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleGiteaAppCallback_Success drives the real pendingState: it
// starts a connect first (to get a real, valid state value), then
// completes the callback with it, and asserts the exchanged tokens
// landed in secrets, never in the redirect itself.
func TestHandleGiteaAppCallback_Success(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{
		exchangeTokens: giteaapp.Tokens{AccessToken: "at-1", RefreshToken: "rt-1", ExpiresAt: time.Now().Add(2 * time.Hour)},
	}
	secrets := newFakeGiteaAppSecrets()
	rt, db := newTestRouterWithGiteaApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)
	setPrimaryDomain(t, db)
	if err := secrets.SetValue(context.Background(), store.GiteaAppSecretsKey(), giteaAppClientSecretKey, "csecret"); err != nil {
		t.Fatalf("seed client_secret: %v", err)
	}

	startRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(startRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/connect", ""))
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/callback?code=the-code&state="+state, ""))
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
	rt.Handler().ServeHTTP(statusRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app", ""))
	if !strings.Contains(statusRec.Body.String(), `"authorized":true`) {
		t.Errorf("status after callback = %s, want authorized:true", statusRec.Body.String())
	}
}

func TestGiteaAccessToken_RefreshesNearExpiry(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{
		refreshTokens: giteaapp.Tokens{AccessToken: "at-fresh", RefreshToken: "rt-fresh", ExpiresAt: time.Now().Add(2 * time.Hour)},
	}
	secrets := newFakeGiteaAppSecrets()
	rt, db := newTestRouterWithGiteaApp(t, secrets, fakeClient)
	ctx := context.Background()
	seedGiteaAppConnection(t, db)

	key := store.GiteaAppSecretsKey()
	mustSetGiteaSecret(t, secrets, key, giteaAppClientSecretKey, "csecret")
	mustSetGiteaSecret(t, secrets, key, giteaAppAccessTokenKey, "at-stale")
	mustSetGiteaSecret(t, secrets, key, giteaAppRefreshTokenKey, "rt-stale")
	// Already past expiry, forcing the refresh path.
	mustSetGiteaSecret(t, secrets, key, giteaAppTokenExpiresKey, "1")

	_, token, err := rt.giteaAccessToken(ctx)
	if err != nil {
		t.Fatalf("giteaAccessToken() error = %v", err)
	}
	if !fakeClient.refreshCalled {
		t.Error("RefreshToken was not called for a near-expiry token")
	}
	if token != "at-fresh" {
		t.Errorf("token = %q, want the freshly refreshed at-fresh", token)
	}
}

func TestGiteaAccessToken_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	seedGiteaAppConnection(t, db)

	_, _, err := rt.giteaAccessToken(context.Background())
	if err == nil {
		t.Fatal("giteaAccessToken() error = nil, want errGiteaAppNotAuthorized")
	}
}

func mustSetGiteaSecret(t *testing.T, s *fakeGiteaAppSecrets, key, envKey, val string) {
	t.Helper()
	if err := s.SetValue(context.Background(), key, envKey, val); err != nil {
		t.Fatalf("seed %s: %v", envKey, err)
	}
}
