package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBitbucketAppSecrets is a hand-written fake for BitbucketAppSecrets,
// the same pattern fakeGitLabAppSecrets already establishes.
type fakeBitbucketAppSecrets struct {
	mu     sync.Mutex
	values map[string]string

	setErr, resolveErr, existsErr error
}

func newFakeBitbucketAppSecrets() *fakeBitbucketAppSecrets {
	return &fakeBitbucketAppSecrets{values: map[string]string{}}
}

func (f *fakeBitbucketAppSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[serviceName+"|"+envKey] = plaintext
	return nil
}

func (f *fakeBitbucketAppSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[serviceName+"|"+envKey]
	if !ok {
		return "", errFakeSecretNotSet
	}
	return v, nil
}

func (f *fakeBitbucketAppSecrets) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[serviceName+"|"+envKey]
	return ok, nil
}

func (f *fakeBitbucketAppSecrets) DeleteAll(_ context.Context, serviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.values {
		if strings.HasPrefix(k, serviceName+"|") {
			delete(f.values, k)
		}
	}
	return nil
}

// fakeBitbucketAppClient is a hand-written fake for BitbucketAppClient,
// the same convention fakeGitLabAppClient already establishes.
type fakeBitbucketAppClient struct {
	exchangeTokens bitbucketapp.Tokens
	exchangeErr    error
	gotCode        string

	refreshTokens bitbucketapp.Tokens
	refreshErr    error
	refreshCalled bool

	repos    []bitbucketapp.Repo
	reposErr error

	repo    bitbucketapp.Repo
	repoErr error

	branches    []bitbucketapp.Branch
	branchesErr error

	createHookErr  error
	createHookURL  string
	createHookTok  string
	createHookCall bool
}

func (f *fakeBitbucketAppClient) ExchangeCode(_ context.Context, _, _, code string) (bitbucketapp.Tokens, error) {
	f.gotCode = code
	return f.exchangeTokens, f.exchangeErr
}

func (f *fakeBitbucketAppClient) RefreshToken(_ context.Context, _, _, _ string) (bitbucketapp.Tokens, error) {
	f.refreshCalled = true
	return f.refreshTokens, f.refreshErr
}

func (f *fakeBitbucketAppClient) ListRepos(_ context.Context, _ string) ([]bitbucketapp.Repo, error) {
	return f.repos, f.reposErr
}

func (f *fakeBitbucketAppClient) GetRepo(_ context.Context, _, _ string) (bitbucketapp.Repo, error) {
	return f.repo, f.repoErr
}

func (f *fakeBitbucketAppClient) ListBranches(_ context.Context, _, _ string) ([]bitbucketapp.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeBitbucketAppClient) CreateRepoWebhook(_ context.Context, _, _, hookURL, secret string) error {
	f.createHookCall = true
	f.createHookURL = hookURL
	f.createHookTok = secret
	return f.createHookErr
}

func newTestRouterWithBitbucketApp(t *testing.T, secrets BitbucketAppSecrets, client BitbucketAppClient) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithBitbucketAppSecrets(secrets))
	rt.bitbucketAppClient = client
	return rt, db
}

func TestHandleGetBitbucketAppStatus_NotConnected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"connected":false`) {
		t.Errorf("body = %s, want connected:false", rec.Body.String())
	}
}

func TestHandleConnectBitbucketApp_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBitbucketAppSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/bitbucket-app",
		`{"key":"k","secret":"s"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleConnectBitbucketApp_MissingFields(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"missing key", `{"secret":"s"}`},
		{"missing secret", `{"key":"k"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/bitbucket-app", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleConnectBitbucketApp_Success(t *testing.T) {
	secrets := newFakeBitbucketAppSecrets()
	rt, db := newTestRouterWithBitbucketApp(t, secrets, &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/bitbucket-app",
		`{"key":"the-key","secret":"the-secret"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "the-secret") {
		t.Errorf("body leaks secret: %s", rec.Body.String())
	}

	statusRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(statusRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app", ""))
	body := statusRec.Body.String()
	if !strings.Contains(body, `"connected":true`) {
		t.Errorf("status body = %s, want connected:true", body)
	}
	if !strings.Contains(body, `"key":"the-key"`) {
		t.Errorf("status body = %s, want key=the-key", body)
	}
	if !strings.Contains(body, `"authorized":false`) {
		t.Errorf("status body = %s, want authorized:false (no oauth flow completed yet)", body)
	}

	got, err := secrets.Resolve(context.Background(), store.BitbucketAppSecretsKey(), bitbucketAppSecretKey)
	if err != nil || got != "the-secret" {
		t.Errorf("stored secret = %q, err = %v, want the-secret", got, err)
	}
}

func TestHandleDisconnectBitbucketApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/bitbucket-app", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (nothing connected)", rec.Code)
	}
}

func TestHandleDisconnectBitbucketApp_ClearsSecrets(t *testing.T) {
	secrets := newFakeBitbucketAppSecrets()
	rt, db := newTestRouterWithBitbucketApp(t, secrets, &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveBitbucketAppConnection(ctx, store.BitbucketAppConnection{
		Key: "the-key", CreatedAt: "2026-08-20T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := secrets.SetValue(ctx, store.BitbucketAppSecretsKey(), bitbucketAppAccessTokenKey, "at-1"); err != nil {
		t.Fatalf("seed access token: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/bitbucket-app", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := secrets.Resolve(ctx, store.BitbucketAppSecretsKey(), bitbucketAppAccessTokenKey); err == nil {
		t.Error("access_token still resolvable after disconnect, want it deleted")
	}
}

func TestBitbucketAppRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/bitbucket-app"},
		{http.MethodPut, "/api/v1/bitbucket-app"},
		{http.MethodDelete, "/api/v1/bitbucket-app"},
		{http.MethodGet, "/api/v1/bitbucket-app/connect"},
		{http.MethodGet, "/api/v1/bitbucket-app/repos"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401 for an unauthenticated request", r.method, r.path, rec.Code)
		}
	}
}

func TestBitbucketAppRoutes_PlainWriteSensitiveTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "bb-write-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_bbws", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWriteSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/bitbucket-app"},
		{http.MethodPut, "/api/v1/bitbucket-app"},
		{http.MethodDelete, "/api/v1/bitbucket-app"},
		{http.MethodGet, "/api/v1/bitbucket-app/connect"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403 (AbilityWriteSensitive must not reach an AbilityRoot route)", r.method, r.path, rec.Code)
		}
	}
}
