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

// fakeRegistrySecrets is a hand-written fake for RegistrySecrets, the
// same pattern fakeCloudflareTunnelSecrets already establishes for the
// structurally similar interface, plus Resolve since
// toRegistrySettingsResource's caller (this file's own tests) never
// needs it directly, but RegistrySecrets' real implementation
// (*secrets.Manager) always has it available.
type fakeRegistrySecrets struct {
	err    error
	values map[string]string
}

func newFakeRegistrySecrets() *fakeRegistrySecrets {
	return &fakeRegistrySecrets{values: map[string]string{}}
}

func (f *fakeRegistrySecrets) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeRegistrySecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeRegistrySecrets) DeleteAll(_ context.Context, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.values = map[string]string{}
	return nil
}

func newTestRouterWithRegistrySecrets(t *testing.T, s RegistrySecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRegistrySecrets(s)), db
}

func TestHandleGetRegistrySettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/registry", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got registrySettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.HasCredentials {
		t.Errorf("HasCredentials = true, want false with no secrets configured")
	}
	if got.Status != "stopped" {
		t.Errorf("Status = %q, want stopped", got.Status)
	}
}

func TestHandleGetRegistrySettings_NeverReturnsPasswordValue(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry.example"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/registry", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got registrySettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Password != "" {
		t.Errorf("GET response contains a password, want empty (write-once, only on the generating PUT)")
	}
	if !got.HasCredentials {
		t.Error("HasCredentials = false, want true after enabling")
	}
	if !got.Enabled || got.Host != "registry.example" {
		t.Errorf("got = %+v, want Enabled=true Host=registry.example", got)
	}
}

func TestHandleUpdateRegistrySettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithRegistrySecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry.example"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateRegistrySettings_EnableWithoutHostRejected(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandleUpdateRegistrySettings_FirstEnable_GeneratesAndReturnsPassword
// proves the one moment a platform-generated password ever appears in a
// response: the PUT call that first enables the registry with no
// existing credentials.
func TestHandleUpdateRegistrySettings_FirstEnable_GeneratesAndReturnsPassword(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry.example"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got registrySettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Password == "" {
		t.Fatal("Password is empty, want a generated password on the first enable")
	}
	if got.Username != registryUsername {
		t.Errorf("Username = %q, want %q", got.Username, registryUsername)
	}

	settings, err := db.GetRegistrySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if !settings.Enabled || settings.Host != "registry.example" {
		t.Errorf("stored settings = %+v, want Enabled=true Host=registry.example", settings)
	}
	if s.values[store.RegistryPasswordEnvKey] != got.Password {
		t.Errorf("stored password = %q, want it to match the returned one %q", s.values[store.RegistryPasswordEnvKey], got.Password)
	}
}

// TestHandleUpdateRegistrySettings_SecondEnable_NeverRegeneratesPassword
// proves a second PUT (e.g. just changing Host, or toggling Enabled back
// on) with credentials already present leaves the existing password
// untouched and never echoes it again.
func TestHandleUpdateRegistrySettings_SecondEnable_NeverRegeneratesPassword(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry.example"}`))
	var firstResp registrySettingsResource
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	originalPassword := s.values[store.RegistryPasswordEnvKey]
	if originalPassword == "" {
		t.Fatal("expected a password to have been generated on first enable")
	}

	second := httptest.NewRecorder()
	rt.Handler().ServeHTTP(second, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry2.example"}`))
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", second.Code, http.StatusOK, second.Body.String())
	}

	var secondResp registrySettingsResource
	if err := json.Unmarshal(second.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if secondResp.Password != "" {
		t.Errorf("Password = %q, want empty on a call that isn't the first enable", secondResp.Password)
	}
	if s.values[store.RegistryPasswordEnvKey] != originalPassword {
		t.Errorf("stored password changed from %q to %q, want unchanged", originalPassword, s.values[store.RegistryPasswordEnvKey])
	}

	settings, err := db.GetRegistrySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if settings.Host != "registry2.example" {
		t.Errorf("Host = %q, want registry2.example (still updatable independent of credentials)", settings.Host)
	}
}

func TestHandleDisableRegistry_ClearsCredentialsAndDisables(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry.example"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/registry", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetRegistrySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want false after disable")
	}
	if _, ok := s.values[store.RegistryPasswordEnvKey]; ok {
		t.Errorf("password still present after disable")
	}
}

func TestHandleDisableRegistry_Idempotent(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/registry", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-disabled registry, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleDisableRegistry_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithRegistrySecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/registry", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

// TestHandleUpdateRegistrySettings_PlainWriteToken_Forbidden proves PUT
// /settings/registry sits behind AbilityRoot, not just AbilityWrite,
// mirroring TestHandleUpdateCloudflareTunnelSettings_PlainWriteToken_Forbidden.
func TestHandleUpdateRegistrySettings_PlainWriteToken_Forbidden(t *testing.T) {
	s := newFakeRegistrySecrets()
	rt, db := newTestRouterWithRegistrySecrets(t, s)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/registry", strings.NewReader(`{"enabled":true,"host":"registry.example"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the registry settings write", rec.Code, http.StatusForbidden)
	}

	settings, err := db.GetRegistrySettings(ctx)
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want unchanged (false) after a rejected request")
	}
}

// TestHandleUpdateRegistrySettings_RealSecretsManager_RoundTripsThroughEncryption
// is the whole-chain proof, mirroring
// TestHandleUpdateCloudflareTunnelSettings_RealSecretsManager_RoundTripsThroughEncryption.
func TestHandleUpdateRegistrySettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithRegistrySecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/registry", `{"enabled":true,"host":"registry.example"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp registrySettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	ctx := context.Background()
	key := store.RegistrySettingsSecretsKey()

	gotPassword, err := secretsManager.Resolve(ctx, key, store.RegistryPasswordEnvKey)
	if err != nil {
		t.Fatalf("Resolve(password) error = %v", err)
	}
	if gotPassword != resp.Password {
		t.Errorf("Resolve(password) = %q, want %q", gotPassword, resp.Password)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.RegistryPasswordEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(password) error = %v", err)
	}
	if strings.Contains(string(ciphertext), resp.Password) {
		t.Error("password ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestRegistryRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/settings/registry"},
		{http.MethodPut, "/api/v1/settings/registry"},
		{http.MethodDelete, "/api/v1/settings/registry"},
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
