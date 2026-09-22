package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeGiteaAppSecrets is a hand-written fake for GiteaAppSecrets, the
// same pattern fakeGitLabAppSecrets already establishes.
type fakeGiteaAppSecrets struct {
	mu     sync.Mutex
	values map[string]string

	setErr, resolveErr, existsErr error
}

func newFakeGiteaAppSecrets() *fakeGiteaAppSecrets {
	return &fakeGiteaAppSecrets{values: map[string]string{}}
}

func (f *fakeGiteaAppSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[serviceName+"|"+envKey] = plaintext
	return nil
}

func (f *fakeGiteaAppSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
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

func (f *fakeGiteaAppSecrets) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[serviceName+"|"+envKey]
	return ok, nil
}

func (f *fakeGiteaAppSecrets) DeleteAll(_ context.Context, serviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.values {
		if strings.HasPrefix(k, serviceName+"|") {
			delete(f.values, k)
		}
	}
	return nil
}

// fakeGiteaAppClient is a hand-written fake for GiteaAppClient, every
// method's return value set directly on the struct before use, the same
// convention fakeGitLabAppClient already establishes.
type fakeGiteaAppClient struct {
	exchangeTokens giteaapp.Tokens
	exchangeErr    error
	gotCode        string

	refreshTokens giteaapp.Tokens
	refreshErr    error
	refreshCalled bool

	repos    []giteaapp.Repo
	reposErr error

	repo    giteaapp.Repo
	repoErr error

	branches    []giteaapp.Branch
	branchesErr error

	createHookErr  error
	createHookURL  string
	createHookTok  string
	createHookCall bool
}

func (f *fakeGiteaAppClient) ExchangeCode(_ context.Context, _, _, _, _, code string) (giteaapp.Tokens, error) {
	f.gotCode = code
	return f.exchangeTokens, f.exchangeErr
}

func (f *fakeGiteaAppClient) RefreshToken(_ context.Context, _, _, _, _ string) (giteaapp.Tokens, error) {
	f.refreshCalled = true
	return f.refreshTokens, f.refreshErr
}

func (f *fakeGiteaAppClient) ListRepos(_ context.Context, _, _ string) ([]giteaapp.Repo, error) {
	return f.repos, f.reposErr
}

func (f *fakeGiteaAppClient) GetRepo(_ context.Context, _, _, _ string) (giteaapp.Repo, error) {
	return f.repo, f.repoErr
}

func (f *fakeGiteaAppClient) ListBranches(_ context.Context, _, _, _ string) ([]giteaapp.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeGiteaAppClient) CreateRepoWebhook(_ context.Context, _, _, _, hookURL, secret string) error {
	f.createHookCall = true
	f.createHookURL = hookURL
	f.createHookTok = secret
	return f.createHookErr
}

func newTestRouterWithGiteaApp(t *testing.T, secrets GiteaAppSecrets, client GiteaAppClient) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGiteaAppSecrets(secrets))
	rt.giteaAppClient = client
	return rt, db
}

func TestHandleGetGiteaAppStatus_NotConnected(t *testing.T) {
	assertProviderStatusNotConnected(t, "/api/v1/gitea-app")
}

func TestHandleConnectGiteaApp_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGiteaAppSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/gitea-app",
		`{"instance_url":"https://git.example.com","client_id":"cid","client_secret":"csecret"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleConnectGiteaApp_MissingFields(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"missing instance_url", `{"client_id":"cid","client_secret":"csecret"}`},
		{"missing client_id", `{"instance_url":"https://git.example.com","client_secret":"csecret"}`},
		{"missing client_secret", `{"instance_url":"https://git.example.com","client_id":"cid"}`},
		{"non-http scheme", `{"instance_url":"ftp://git.example.com","client_id":"cid","client_secret":"csecret"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/gitea-app", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleConnectGiteaApp_Success(t *testing.T) {
	secrets := newFakeGiteaAppSecrets()
	rt, db := newTestRouterWithGiteaApp(t, secrets, &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/gitea-app",
		`{"instance_url":"https://git.example.com/","client_id":"cid","client_secret":"csecret"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "csecret") {
		t.Errorf("body leaks client_secret: %s", rec.Body.String())
	}

	statusRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(statusRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app", ""))
	// instance_url: trailing slash from the request is stripped, not stored verbatim.
	assertBodyContainsAll(t, statusRec.Body.String(), `"connected":true`, `"instance_url":"https://git.example.com"`, `"authorized":false`)

	got, err := secrets.Resolve(context.Background(), store.GiteaAppSecretsKey(), giteaAppClientSecretKey)
	if err != nil || got != "csecret" {
		t.Errorf("stored client_secret = %q, err = %v, want csecret", got, err)
	}
}

func TestHandleDisconnectGiteaApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/gitea-app", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (nothing connected)", rec.Code)
	}
}

func TestHandleDisconnectGiteaApp_ClearsSecrets(t *testing.T) {
	secrets := newFakeGiteaAppSecrets()
	rt, db := newTestRouterWithGiteaApp(t, secrets, &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveGiteaAppConnection(ctx, store.GiteaAppConnection{
		InstanceURL: "https://git.example.com", ClientID: "cid", CreatedAt: "2026-08-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := secrets.SetValue(ctx, store.GiteaAppSecretsKey(), giteaAppAccessTokenKey, "at-1"); err != nil {
		t.Fatalf("seed access token: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/gitea-app", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := secrets.Resolve(ctx, store.GiteaAppSecretsKey(), giteaAppAccessTokenKey); err == nil {
		t.Error("access_token still resolvable after disconnect, want it deleted")
	}
}

func TestGiteaAppRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertProviderRoutesRequireAuth(t, rt, []providerRouteCase{
		{method: http.MethodGet, path: "/api/v1/gitea-app"},
		{method: http.MethodPut, path: "/api/v1/gitea-app"},
		{method: http.MethodDelete, path: "/api/v1/gitea-app"},
		{method: http.MethodGet, path: "/api/v1/gitea-app/connect"},
		{method: http.MethodGet, path: "/api/v1/gitea-app/repos"},
	})
}

func TestGiteaAppRoutes_PlainWriteSensitiveTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)

	const plaintext = "write-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok_ws_gitea", plaintext, []string{AbilityWriteSensitive}, []providerRouteCase{
		{method: http.MethodGet, path: "/api/v1/gitea-app"},
		{method: http.MethodPut, path: "/api/v1/gitea-app"},
		{method: http.MethodDelete, path: "/api/v1/gitea-app"},
		{method: http.MethodGet, path: "/api/v1/gitea-app/connect"},
	})
}
