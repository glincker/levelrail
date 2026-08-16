package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeGitHubAppSecrets is a hand-written fake for GitHubAppSecrets, the
// same pattern fakeSecretSetter (secrets_test.go) and
// fakeBackupSecretsSetter (backup_targets_test.go) already establish,
// extended with Resolve since this is the one feature that reads
// secrets back through internal/api itself (see GitHubAppSecrets's own
// doc comment for why).
type fakeGitHubAppSecrets struct {
	mu         sync.Mutex
	values     map[string]string
	setErr     error
	resolveErr error
}

func newFakeGitHubAppSecrets() *fakeGitHubAppSecrets {
	return &fakeGitHubAppSecrets{values: map[string]string{}}
}

func (f *fakeGitHubAppSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[serviceName+"|"+envKey] = plaintext
	return nil
}

func (f *fakeGitHubAppSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
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

func (f *fakeGitHubAppSecrets) DeleteAll(_ context.Context, serviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.values {
		if strings.HasPrefix(k, serviceName+"|") {
			delete(f.values, k)
		}
	}
	return nil
}

var errFakeSecretNotSet = &fakeSecretNotSetError{}

type fakeSecretNotSetError struct{}

func (*fakeSecretNotSetError) Error() string { return "fake: secret not set" }

// fakeGitHubAppClient is a hand-written fake for GitHubAppClient: every
// method's return value is set directly on the struct by the test
// before use, the same "no mocking framework, plain struct fields"
// convention this package's other fakes already use.
type fakeGitHubAppClient struct {
	exchangeCreds githubapp.Credentials
	exchangeErr   error
	gotCode       string

	installation    githubapp.InstallationInfo
	installationErr error
	gotAppJWT       string
	gotInstallID    int64

	mintToken githubapp.InstallationToken
	mintErr   error

	repos    []githubapp.Repo
	reposErr error

	branches    []githubapp.Branch
	branchesErr error
}

func (f *fakeGitHubAppClient) ExchangeManifestCode(_ context.Context, code string) (githubapp.Credentials, error) {
	f.gotCode = code
	return f.exchangeCreds, f.exchangeErr
}

func (f *fakeGitHubAppClient) GetInstallation(_ context.Context, appJWT string, installationID int64) (githubapp.InstallationInfo, error) {
	f.gotAppJWT = appJWT
	f.gotInstallID = installationID
	return f.installation, f.installationErr
}

func (f *fakeGitHubAppClient) MintInstallationToken(_ context.Context, appJWT string, installationID int64) (githubapp.InstallationToken, error) {
	f.gotAppJWT = appJWT
	f.gotInstallID = installationID
	return f.mintToken, f.mintErr
}

func (f *fakeGitHubAppClient) ListInstallationRepos(_ context.Context, _ string) ([]githubapp.Repo, error) {
	return f.repos, f.reposErr
}

func (f *fakeGitHubAppClient) ListBranches(_ context.Context, _, _, _ string) ([]githubapp.Branch, error) {
	return f.branches, f.branchesErr
}

func TestHandleGetGitHubAppStatus_NotConnected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"connected":false`) {
		t.Errorf("body = %s, want connected:false", rec.Body.String())
	}
}

func TestHandleGetGitHubAppStatus_Connected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"connected":true`) {
		t.Errorf("body = %s, want connected:true", body)
	}
	if !strings.Contains(body, `"installed":false`) {
		t.Errorf("body = %s, want installed:false (no installation recorded yet)", body)
	}
	// Never leaks client_secret/webhook_secret/private_key: none of
	// those field names appear anywhere in status's own resource type,
	// but this asserts on the actual serialized response too, not just
	// the struct definition.
	for _, secretField := range []string{"client_secret", "webhook_secret", "private_key"} {
		if strings.Contains(body, secretField) {
			t.Errorf("body contains %q, status response must never include secret fields", secretField)
		}
	}
}

func TestHandleDisconnectGitHubApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/github-app", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}

	statusRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(statusRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app", ""))
	if !strings.Contains(statusRec.Body.String(), `"connected":false`) {
		t.Errorf("status after disconnect = %s, want connected:false", statusRec.Body.String())
	}
}

// TestHandleDisconnectGitHubApp_ClearsSecrets proves disconnect really
// deletes the App's credentials, not just the connection row: a token
// set under store.GitHubAppSecretsKey() before disconnect must be
// unresolvable after it, closing the gap where a "disconnected" App's
// private key would otherwise remain decryptable indefinitely.
func TestHandleDisconnectGitHubApp_ClearsSecrets(t *testing.T) {
	secrets := newFakeGitHubAppSecrets()
	rt, db := newTestRouterWithGitHubApp(t, secrets, &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveGitHubAppConnection(ctx, store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	secretsKey := store.GitHubAppSecretsKey()
	if err := secrets.SetValue(ctx, secretsKey, "private_key", "test-pem"); err != nil {
		t.Fatalf("seed private_key: %v", err)
	}
	if err := secrets.SetValue(ctx, secretsKey, "client_secret", "test-secret"); err != nil {
		t.Fatalf("seed client_secret: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/github-app", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := secrets.Resolve(ctx, secretsKey, "private_key"); err == nil {
		t.Error("private_key still resolvable after disconnect, want it deleted")
	}
	if _, err := secrets.Resolve(ctx, secretsKey, "client_secret"); err == nil {
		t.Error("client_secret still resolvable after disconnect, want it deleted")
	}
}

func TestHandleDisconnectGitHubApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/github-app", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (nothing connected)", rec.Code)
	}
}

// TestGitHubAppRoutes_PlainTokenForbidden proves every GitHub App
// connection-management route really sits behind AbilityRoot, not just
// any authenticated caller: a token scoped only to AbilityWriteSensitive
// (the tier immediately below AbilityRoot that a naive reading might
// mistake for "sensitive enough") must still be rejected with 403. The
// same "prove it with a real request, not just router registration"
// pattern TestHandleUpdateIngressSettings_PlainWriteToken_Forbidden and
// TestHandleSetAppNode_PlainWriteToken_Forbidden already establish.
func TestGitHubAppRoutes_PlainTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "write-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_ws", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWriteSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	routes := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/github-app"},
		{http.MethodDelete, "/api/v1/github-app"},
		{http.MethodGet, "/api/v1/github-app/register/start"},
		{http.MethodGet, "/api/v1/github-app/callback?code=x&state=y"},
		{http.MethodGet, "/api/v1/github-app/installed?installation_id=1"},
	}
	for _, rt2 := range routes {
		req := httptest.NewRequest(rt2.method, rt2.path, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403 (AbilityWriteSensitive must not reach an AbilityRoot route)", rt2.method, rt2.path, rec.Code)
		}
	}
}

// TestGitHubAppRepoRoutes_PlainReadTokenForbidden proves the repo/branch
// listing routes sit behind AbilityReadSensitive, not the plain
// AbilityRead tier: a token scoped only to AbilityRead must be rejected.
func TestGitHubAppRepoRoutes_PlainReadTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "read-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	routes := []string{
		"/api/v1/github-app/repos",
		"/api/v1/github-app/repos/acme/widgets/branches",
	}
	for _, path := range routes {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s: status = %d, want 403 (AbilityRead must not reach an AbilityReadSensitive route)", path, rec.Code)
		}
	}
}

func TestGitHubAppRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/github-app"},
		{http.MethodDelete, "/api/v1/github-app"},
		{http.MethodGet, "/api/v1/github-app/register/start"},
		{http.MethodGet, "/api/v1/github-app/repos"},
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

func TestGitHubAppRegistrationState_ConsumeSingleUse(t *testing.T) {
	s := newGitHubAppRegistrationState()

	state, err := s.begin()
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if state == "" {
		t.Fatal("begin() returned an empty state")
	}

	if !s.consume(state) {
		t.Fatal("consume(state) = false on first use, want true")
	}
	if s.consume(state) {
		t.Fatal("consume(state) = true on second use, want false (single-use)")
	}
}

func TestGitHubAppRegistrationState_ConsumeWrongValue(t *testing.T) {
	s := newGitHubAppRegistrationState()
	if _, err := s.begin(); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if s.consume("not-the-real-state") {
		t.Error("consume(wrong value) = true, want false")
	}
	// A wrong guess must not have burned the real pending state.
	realState, err := s.begin()
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if !s.consume(realState) {
		t.Error("consume(real state) after a wrong guess = false, want true")
	}
}

func TestGitHubAppRegistrationState_Expired(t *testing.T) {
	s := newGitHubAppRegistrationState()
	state, err := s.begin()
	if err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	s.mu.Lock()
	s.expiresAt = time.Now().Add(-time.Second)
	s.mu.Unlock()

	if s.consume(state) {
		t.Error("consume(state) after expiry = true, want false")
	}
}

func TestGitHubAppRegistrationState_EmptyInputsRejected(t *testing.T) {
	s := newGitHubAppRegistrationState()
	if s.consume("") {
		t.Error("consume(\"\") on a fresh state store = true, want false")
	}
	if _, err := s.begin(); err != nil {
		t.Fatalf("begin() error = %v", err)
	}
	if s.consume("") {
		t.Error("consume(\"\") with a real pending state = true, want false")
	}
}
