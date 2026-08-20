package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDomainBasicAuthSecrets is a hand-written fake for
// DomainBasicAuthSecrets, the same pattern fakeCloudflareTunnelSecrets
// already establishes for the structurally identical interface.
type fakeDomainBasicAuthSecrets struct {
	err    error
	values map[string]string
}

func newFakeDomainBasicAuthSecrets() *fakeDomainBasicAuthSecrets {
	return &fakeDomainBasicAuthSecrets{values: map[string]string{}}
}

func (f *fakeDomainBasicAuthSecrets) SetValue(_ context.Context, serviceName, _, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[serviceName] = plaintext
	return nil
}

func (f *fakeDomainBasicAuthSecrets) Exists(_ context.Context, serviceName, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[serviceName]
	return ok, nil
}

func (f *fakeDomainBasicAuthSecrets) DeleteAll(_ context.Context, serviceName string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.values, serviceName)
	return nil
}

func newTestRouterWithDomainBasicAuthSecrets(t *testing.T, s DomainBasicAuthSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithDomainBasicAuthSecrets(s)), db
}

// seedAppWithDomain creates a desired service named "web" claiming
// app.example.com, the fixture every handler test below needs since
// requireOwnedDomain 404s for an app or domain that doesn't exist.
func seedAppWithDomain(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "img:v1", Port: 8080, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
}

func TestHandleGetDomainBasicAuth_NoneConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/auth", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainBasicAuthResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled || got.HasPassword || got.Username != "" {
		t.Errorf("got = %+v, want all zero values before any basic auth is set", got)
	}
}

func TestHandleGetDomainBasicAuth_DomainNotOwnedByApp(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/auth", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for a domain the app does not own", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetDomainBasicAuth_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/domains/app.example.com/auth", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDomainBasicAuth_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDomainBasicAuthSecrets
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"username":"operator","password":"hunter2"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleSetDomainBasicAuth_MissingUsernameRejected(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"password":"hunter2"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDomainBasicAuth_EnableWithoutPasswordRejected(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"username":"operator"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDomainBasicAuth_Success(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"username":"operator","password":"hunter2"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainBasicAuthResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled || !got.HasPassword || got.Username != "operator" {
		t.Errorf("got = %+v, want Enabled=true HasPassword=true Username=operator", got)
	}

	auth, found, err := db.GetDomainBasicAuth(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	}
	if !found || auth.Username != "operator" {
		t.Errorf("GetDomainBasicAuth() = (%+v, %v), want Username=operator, found=true", auth, found)
	}
	if s.values[store.DomainBasicAuthSecretsKey("app.example.com")] != "hunter2" {
		t.Errorf("stored password = %q, want hunter2", s.values[store.DomainBasicAuthSecretsKey("app.example.com")])
	}
}

func TestHandleSetDomainBasicAuth_NeverReturnsPasswordValue(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"username":"operator","password":"super-secret-password"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "super-secret-password") {
		t.Errorf("PUT response contains the raw password: %s", rec.Body.String())
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/auth", ""))
	if strings.Contains(getRec.Body.String(), "super-secret-password") {
		t.Errorf("GET response contains the raw password: %s", getRec.Body.String())
	}
}

func TestHandleSetDomainBasicAuth_OmittedPasswordLeavesExistingIntact(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	first := `{"username":"operator","password":"original"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", first))

	second := `{"username":"operator2"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", second))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if s.values[store.DomainBasicAuthSecretsKey("app.example.com")] != "original" {
		t.Errorf("stored password = %q, want it left untouched at 'original'", s.values[store.DomainBasicAuthSecretsKey("app.example.com")])
	}

	auth, found, err := db.GetDomainBasicAuth(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	}
	if !found || auth.Username != "operator2" {
		t.Errorf("GetDomainBasicAuth() = (%+v, %v), want the username to still update to operator2", auth, found)
	}
}

func TestHandleClearDomainBasicAuth_RemovesUsernameAndPassword(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", `{"username":"operator","password":"hunter2"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/auth", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if _, found, err := db.GetDomainBasicAuth(context.Background(), "app.example.com"); err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after clearing")
	}
	if _, ok := s.values[store.DomainBasicAuthSecretsKey("app.example.com")]; ok {
		t.Errorf("password still present after clearing")
	}
}

func TestHandleClearDomainBasicAuth_Idempotent(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/auth", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-cleared domain, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestDomainBasicAuthWriteRoutes_PlainWriteToken_Forbidden proves PUT/
// DELETE .../domains/{domain}/auth sit behind AbilityRoot, not just
// AbilityWrite, mirroring
// TestHandleUpdateCloudflareTunnelSettings_PlainWriteToken_Forbidden.
func TestDomainBasicAuthWriteRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	s := newFakeDomainBasicAuthSecrets()
	rt, db := newTestRouterWithDomainBasicAuthSecrets(t, s)
	seedAppWithDomain(t, db)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", strings.NewReader(`{"username":"operator","password":"hunter2"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the domain basic auth write", rec.Code, http.StatusForbidden)
	}
}

func TestHandleUpdateDomainBasicAuth_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	seedAppWithDomain(t, db)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithDomainBasicAuthSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const password = "correct-horse-battery-staple" //nolint:gosec // fake fixture, not a real credential
	body := `{"username":"operator","password":"` + password + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.DomainBasicAuthSecretsKey("app.example.com")

	gotPassword, err := secretsManager.Resolve(ctx, key, store.DomainBasicAuthPasswordEnvKey)
	if err != nil {
		t.Fatalf("Resolve(password) error = %v", err)
	}
	if gotPassword != password {
		t.Errorf("Resolve(password) = %q, want %q", gotPassword, password)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.DomainBasicAuthPasswordEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(password) error = %v", err)
	}
	if strings.Contains(string(ciphertext), password) {
		t.Error("password ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestDomainBasicAuthRoutes_RequireAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/apps/web/domains/app.example.com/auth"},
		{http.MethodPut, "/api/v1/apps/web/domains/app.example.com/auth"},
		{http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/auth"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}
