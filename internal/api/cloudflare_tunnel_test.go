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

// fakeCloudflareTunnelSecrets is a hand-written fake for
// CloudflareTunnelSecrets, the same pattern fakeEmailSecretsStore
// (email_settings_test.go) already establishes for the structurally
// similar interface.
type fakeCloudflareTunnelSecrets struct {
	err    error
	values map[string]string
}

func newFakeCloudflareTunnelSecrets() *fakeCloudflareTunnelSecrets {
	return &fakeCloudflareTunnelSecrets{values: map[string]string{}}
}

func (f *fakeCloudflareTunnelSecrets) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeCloudflareTunnelSecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeCloudflareTunnelSecrets) DeleteAll(_ context.Context, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.values = map[string]string{}
	return nil
}

func newTestRouterWithCloudflareTunnelSecrets(t *testing.T, s CloudflareTunnelSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithCloudflareTunnelSecrets(s)), db
}

func TestHandleGetCloudflareTunnelSettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/cloudflare-tunnel", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got cloudflareTunnelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.HasToken {
		t.Errorf("HasToken = true, want false with no secrets configured")
	}
	if got.Status != "disconnected" {
		t.Errorf("Status = %q, want disconnected", got.Status)
	}
}

func TestHandleGetCloudflareTunnelSettings_NeverReturnsTokenValue(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"token":"super-secret-tunnel-token"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", body))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/cloudflare-tunnel", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "super-secret-tunnel-token") {
		t.Errorf("GET response contains the raw token: %s", bodyStr)
	}

	var got cloudflareTunnelResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.HasToken {
		t.Error("HasToken = false, want true after a save with a token")
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestHandleUpdateCloudflareTunnelSettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithCloudflareTunnelSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"token":"tok"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateCloudflareTunnelSettings_EnableWithoutTokenRejected(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateCloudflareTunnelSettings_Success(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"token":"shh"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetCloudflareTunnelSettings(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareTunnelSettings() error = %v", err)
	}
	if !settings.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if s.values[store.CloudflareTunnelTokenEnvKey] != "shh" {
		t.Errorf("stored token = %q, want shh", s.values[store.CloudflareTunnelTokenEnvKey])
	}
}

func TestHandleUpdateCloudflareTunnelSettings_OmittedTokenLeavesExistingIntact(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	first := `{"enabled":true,"token":"original"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", first))

	second := `{"enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", second))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if s.values[store.CloudflareTunnelTokenEnvKey] != "original" {
		t.Errorf("stored token = %q, want it left untouched at 'original'", s.values[store.CloudflareTunnelTokenEnvKey])
	}
}

func TestHandleDisconnectCloudflareTunnel_ClearsTokenAndDisables(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", `{"enabled":true,"token":"tok"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/cloudflare-tunnel", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetCloudflareTunnelSettings(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareTunnelSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want false after disconnect")
	}
	if _, ok := s.values[store.CloudflareTunnelTokenEnvKey]; ok {
		t.Errorf("token still present after disconnect")
	}
}

func TestHandleDisconnectCloudflareTunnel_Idempotent(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/cloudflare-tunnel", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-disconnected tunnel, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestHandleUpdateCloudflareTunnelSettings_PlainWriteToken_Forbidden proves
// PUT /settings/cloudflare-tunnel sits behind AbilityRoot, not just
// AbilityWrite, mirroring TestHandleUpdateEmailSettings_PlainWriteToken_Forbidden.
func TestHandleUpdateCloudflareTunnelSettings_PlainWriteToken_Forbidden(t *testing.T) {
	s := newFakeCloudflareTunnelSecrets()
	rt, db := newTestRouterWithCloudflareTunnelSecrets(t, s)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"enabled":true,"token":"tok"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/cloudflare-tunnel", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the cloudflare tunnel settings write", rec.Code, http.StatusForbidden)
	}

	settings, err := db.GetCloudflareTunnelSettings(ctx)
	if err != nil {
		t.Fatalf("GetCloudflareTunnelSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want unchanged (false) after a rejected request")
	}
}

// TestHandleUpdateCloudflareTunnelSettings_RealSecretsManager_RoundTripsThroughEncryption
// is the whole-chain proof, mirroring
// TestHandleUpdateEmailSettings_RealSecretsManager_RoundTripsThroughEncryption.
func TestHandleUpdateCloudflareTunnelSettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithCloudflareTunnelSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const token = "correct-horse-battery-staple-tunnel" //nolint:gosec // fake fixture, not a real credential
	body := `{"enabled":true,"token":"` + token + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.CloudflareTunnelSecretsKey()

	gotToken, err := secretsManager.Resolve(ctx, key, store.CloudflareTunnelTokenEnvKey)
	if err != nil {
		t.Fatalf("Resolve(token) error = %v", err)
	}
	if gotToken != token {
		t.Errorf("Resolve(token) = %q, want %q", gotToken, token)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.CloudflareTunnelTokenEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(token) error = %v", err)
	}
	if strings.Contains(string(ciphertext), token) {
		t.Error("token ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestCloudflareTunnelRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/settings/cloudflare-tunnel"},
		{http.MethodPut, "/api/v1/settings/cloudflare-tunnel"},
		{http.MethodDelete, "/api/v1/settings/cloudflare-tunnel"},
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
