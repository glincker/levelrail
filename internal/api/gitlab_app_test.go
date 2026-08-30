package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeGitLabAppSecrets is a hand-written fake for GitLabAppSecrets, the
// same pattern fakeGitHubAppSecrets already establishes, extended with
// Exists the way fakeGitSourceSecrets is (this interface needs both).
type fakeGitLabAppSecrets struct {
	mu     sync.Mutex
	values map[string]string

	setErr, resolveErr, existsErr error
}

func newFakeGitLabAppSecrets() *fakeGitLabAppSecrets {
	return &fakeGitLabAppSecrets{values: map[string]string{}}
}

func (f *fakeGitLabAppSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[serviceName+"|"+envKey] = plaintext
	return nil
}

func (f *fakeGitLabAppSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
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

func (f *fakeGitLabAppSecrets) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[serviceName+"|"+envKey]
	return ok, nil
}

func (f *fakeGitLabAppSecrets) DeleteAll(_ context.Context, serviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.values {
		if strings.HasPrefix(k, serviceName+"|") {
			delete(f.values, k)
		}
	}
	return nil
}

// fakeGitLabAppClient is a hand-written fake for GitLabAppClient, every
// method's return value set directly on the struct before use, the same
// convention fakeGitHubAppClient already establishes.
type fakeGitLabAppClient struct {
	exchangeTokens gitlabapp.Tokens
	exchangeErr    error
	gotCode        string

	refreshTokens gitlabapp.Tokens
	refreshErr    error
	refreshCalled bool

	projects    []gitlabapp.Project
	projectsErr error

	project    gitlabapp.Project
	projectErr error

	branches    []gitlabapp.Branch
	branchesErr error

	createHookErr  error
	createHookURL  string
	createHookTok  string
	createHookCall bool
}

func (f *fakeGitLabAppClient) ExchangeCode(_ context.Context, _, _, _, _, code string) (gitlabapp.Tokens, error) {
	f.gotCode = code
	return f.exchangeTokens, f.exchangeErr
}

func (f *fakeGitLabAppClient) RefreshToken(_ context.Context, _, _, _, _ string) (gitlabapp.Tokens, error) {
	f.refreshCalled = true
	return f.refreshTokens, f.refreshErr
}

func (f *fakeGitLabAppClient) ListProjects(_ context.Context, _, _ string) ([]gitlabapp.Project, error) {
	return f.projects, f.projectsErr
}

func (f *fakeGitLabAppClient) GetProject(_ context.Context, _, _ string, _ int64) (gitlabapp.Project, error) {
	return f.project, f.projectErr
}

func (f *fakeGitLabAppClient) ListBranches(_ context.Context, _, _ string, _ int64) ([]gitlabapp.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeGitLabAppClient) CreateProjectWebhook(_ context.Context, _, _ string, _ int64, hookURL, secretToken string) error {
	f.createHookCall = true
	f.createHookURL = hookURL
	f.createHookTok = secretToken
	return f.createHookErr
}

func newTestRouterWithGitLabApp(t *testing.T, secrets GitLabAppSecrets, client GitLabAppClient) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitLabAppSecrets(secrets))
	rt.gitlabAppClient = client
	return rt, db
}

func TestHandleGetGitLabAppStatus_NotConnected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"connected":false`) {
		t.Errorf("body = %s, want connected:false", rec.Body.String())
	}
}

func TestHandleConnectGitLabApp_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGitLabAppSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/gitlab-app",
		`{"instance_url":"https://gitlab.example.com","client_id":"cid","client_secret":"csecret"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleConnectGitLabApp_MissingFields(t *testing.T) {
	rt, db := newTestRouterWithGitLabApp(t, newFakeGitLabAppSecrets(), &fakeGitLabAppClient{})
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"missing instance_url", `{"client_id":"cid","client_secret":"csecret"}`},
		{"missing client_id", `{"instance_url":"https://gitlab.example.com","client_secret":"csecret"}`},
		{"missing client_secret", `{"instance_url":"https://gitlab.example.com","client_id":"cid"}`},
		{"non-http scheme", `{"instance_url":"ftp://gitlab.example.com","client_id":"cid","client_secret":"csecret"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/gitlab-app", tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleConnectGitLabApp_Success(t *testing.T) {
	secrets := newFakeGitLabAppSecrets()
	rt, db := newTestRouterWithGitLabApp(t, secrets, &fakeGitLabAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/gitlab-app",
		`{"instance_url":"https://gitlab.example.com/","client_id":"cid","client_secret":"csecret"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "csecret") {
		t.Errorf("body leaks client_secret: %s", rec.Body.String())
	}

	statusRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(statusRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app", ""))
	body := statusRec.Body.String()
	if !strings.Contains(body, `"connected":true`) {
		t.Errorf("status body = %s, want connected:true", body)
	}
	// Trailing slash from the request is stripped, not stored verbatim.
	if !strings.Contains(body, `"instance_url":"https://gitlab.example.com"`) {
		t.Errorf("status body = %s, want instance_url with no trailing slash", body)
	}
	if !strings.Contains(body, `"authorized":false`) {
		t.Errorf("status body = %s, want authorized:false (no oauth flow completed yet)", body)
	}

	got, err := secrets.Resolve(context.Background(), store.GitLabAppSecretsKey(), gitLabAppClientSecretKey)
	if err != nil || got != "csecret" {
		t.Errorf("stored client_secret = %q, err = %v, want csecret", got, err)
	}
}

func TestHandleDisconnectGitLabApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/gitlab-app", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (nothing connected)", rec.Code)
	}
}

func TestHandleDisconnectGitLabApp_ClearsSecrets(t *testing.T) {
	secrets := newFakeGitLabAppSecrets()
	rt, db := newTestRouterWithGitLabApp(t, secrets, &fakeGitLabAppClient{})
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveGitLabAppConnection(ctx, store.GitLabAppConnection{
		InstanceURL: "https://gitlab.example.com", ClientID: "cid", CreatedAt: "2026-08-16T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := secrets.SetValue(ctx, store.GitLabAppSecretsKey(), gitLabAppAccessTokenKey, "at-1"); err != nil {
		t.Fatalf("seed access token: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/gitlab-app", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := secrets.Resolve(ctx, store.GitLabAppSecretsKey(), gitLabAppAccessTokenKey); err == nil {
		t.Error("access_token still resolvable after disconnect, want it deleted")
	}
}

func TestGitLabAppRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/gitlab-app"},
		{http.MethodPut, "/api/v1/gitlab-app"},
		{http.MethodDelete, "/api/v1/gitlab-app"},
		{http.MethodGet, "/api/v1/gitlab-app/connect"},
		{http.MethodGet, "/api/v1/gitlab-app/projects"},
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

func TestGitLabAppRoutes_PlainWriteSensitiveTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "write-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_ws2", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWriteSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/gitlab-app"},
		{http.MethodPut, "/api/v1/gitlab-app"},
		{http.MethodDelete, "/api/v1/gitlab-app"},
		{http.MethodGet, "/api/v1/gitlab-app/connect"},
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
