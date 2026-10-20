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

// fakeVaultSecrets is a hand-written fake for VaultSecrets, the same
// pattern fakeCloudflareTunnelSecrets already establishes for the
// structurally identical interface.
type fakeVaultSecrets struct {
	err    error
	values map[string]string
}

func newFakeVaultSecrets() *fakeVaultSecrets {
	return &fakeVaultSecrets{values: map[string]string{}}
}

func (f *fakeVaultSecrets) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeVaultSecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeVaultSecrets) DeleteAll(_ context.Context, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.values = map[string]string{}
	return nil
}

func newTestRouterWithVaultSecrets(t *testing.T, s VaultSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithVaultSecrets(s)), db
}

func TestHandleGetVaultSettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/vault", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got vaultSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.HasCredential {
		t.Errorf("HasCredential = true, want false with no secrets configured")
	}
	if got.AuthMethod != store.VaultAuthMethodToken {
		t.Errorf("AuthMethod = %q, want %q", got.AuthMethod, store.VaultAuthMethodToken)
	}
}

func TestHandleGetVaultSettings_NeverReturnsCredentialValue(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"token","credential":"super-secret-vault-token"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/vault", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "super-secret-vault-token") {
		t.Errorf("GET response contains the raw credential: %s", bodyStr)
	}

	var got vaultSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.HasCredential {
		t.Error("HasCredential = false, want true after a save with a credential")
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestHandleUpdateVaultSettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithVaultSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"token","credential":"tok"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateVaultSettings_EnableWithoutCredentialRejected(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"token"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateVaultSettings_EnableWithoutAddressRejected(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"auth_method":"token","credential":"tok"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateVaultSettings_AppRoleWithoutRoleIDRejected(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"approle","credential":"secret-id"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateVaultSettings_InvalidAuthMethodRejected(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":false,"auth_method":"bogus"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateVaultSettings_Success(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"token","mount_path":"kv","credential":"shh"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetVaultSettings(context.Background())
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if !settings.Enabled || settings.Address != "https://vault:8200" || settings.MountPath != "kv" {
		t.Errorf("settings = %+v, want Enabled=true Address=https://vault:8200 MountPath=kv", settings)
	}
	if s.values[store.VaultCredentialEnvKey] != "shh" {
		t.Errorf("stored credential = %q, want shh", s.values[store.VaultCredentialEnvKey])
	}
}

func TestHandleUpdateVaultSettings_AppRoleSuccess(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"approle","role_id":"role-123","credential":"a-secret-id"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetVaultSettings(context.Background())
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if settings.AuthMethod != store.VaultAuthMethodAppRole || settings.RoleID != "role-123" {
		t.Errorf("settings = %+v, want AuthMethod=approle RoleID=role-123", settings)
	}
}

func TestHandleUpdateVaultSettings_OmittedCredentialLeavesExistingIntact(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	first := `{"enabled":true,"address":"https://vault:8200","auth_method":"token","credential":"original"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", first))

	second := `{"enabled":true,"address":"https://vault:8200","auth_method":"token"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", second))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if s.values[store.VaultCredentialEnvKey] != "original" {
		t.Errorf("stored credential = %q, want it left untouched at 'original'", s.values[store.VaultCredentialEnvKey])
	}
}

func TestHandleDisconnectVault_ClearsCredentialAndDisablesButKeepsAddress(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", `{"enabled":true,"address":"https://vault:8200","auth_method":"token","credential":"tok"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/vault", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetVaultSettings(context.Background())
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want false after disconnect")
	}
	if settings.Address != "https://vault:8200" {
		t.Errorf("Address = %q, want it preserved (only Enabled/credential clear on disconnect)", settings.Address)
	}
	if _, ok := s.values[store.VaultCredentialEnvKey]; ok {
		t.Errorf("credential still present after disconnect")
	}
}

func TestHandleDisconnectVault_Idempotent(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/vault", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-disconnected vault, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestHandleUpdateVaultSettings_PlainWriteToken_Forbidden proves PUT
// /settings/vault sits behind AbilityRoot, not just AbilityWrite,
// mirroring TestHandleUpdateCloudflareTunnelSettings_PlainWriteToken_Forbidden.
func TestHandleUpdateVaultSettings_PlainWriteToken_Forbidden(t *testing.T) {
	s := newFakeVaultSecrets()
	rt, db := newTestRouterWithVaultSecrets(t, s)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"token","credential":"tok"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the vault settings write", rec.Code, http.StatusForbidden)
	}

	settings, err := db.GetVaultSettings(ctx)
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want unchanged (false) after a rejected request")
	}
}

// TestHandleUpdateVaultSettings_RealSecretsManager_RoundTripsThroughEncryption
// is the whole-chain proof, mirroring
// TestHandleUpdateCloudflareTunnelSettings_RealSecretsManager_RoundTripsThroughEncryption.
func TestHandleUpdateVaultSettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithVaultSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const token = "correct-horse-battery-staple-vault" //nolint:gosec // fake fixture, not a real credential
	body := `{"enabled":true,"address":"https://vault:8200","auth_method":"token","credential":"` + token + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/vault", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.VaultSecretsKey()

	gotToken, err := secretsManager.Resolve(ctx, key, store.VaultCredentialEnvKey)
	if err != nil {
		t.Fatalf("Resolve(credential) error = %v", err)
	}
	if gotToken != token {
		t.Errorf("Resolve(credential) = %q, want %q", gotToken, token)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.VaultCredentialEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(credential) error = %v", err)
	}
	if strings.Contains(string(ciphertext), token) {
		t.Error("credential ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestVaultSettingsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/settings/vault"},
		{http.MethodPut, "/api/v1/settings/vault"},
		{http.MethodDelete, "/api/v1/settings/vault"},
	})
}
