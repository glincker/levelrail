package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeCloudflareDNSSecrets mirrors fakeCloudflareTunnelSecrets exactly,
// for the structurally identical CloudflareDNSSecrets interface.
type fakeCloudflareDNSSecrets struct {
	err    error
	values map[string]string
}

func newFakeCloudflareDNSSecrets() *fakeCloudflareDNSSecrets {
	return &fakeCloudflareDNSSecrets{values: map[string]string{}}
}

func (f *fakeCloudflareDNSSecrets) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeCloudflareDNSSecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeCloudflareDNSSecrets) DeleteAll(_ context.Context, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.values = map[string]string{}
	return nil
}

func newTestRouterWithCloudflareDNSSecrets(t *testing.T, s CloudflareDNSSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithCloudflareDNSSecrets(s)), db
}

func TestHandleGetCloudflareDNSSettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/cloudflare-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got cloudflareDNSResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.HasToken {
		t.Errorf("HasToken = true, want false with no secrets configured")
	}
}

func TestHandleGetCloudflareDNSSettings_NeverReturnsTokenValue(t *testing.T) {
	s := newFakeCloudflareDNSSecrets()
	rt, db := newTestRouterWithCloudflareDNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"token":"super-secret-dns-token"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", body))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/cloudflare-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "super-secret-dns-token") {
		t.Errorf("GET response contains the raw token: %s", bodyStr)
	}

	var got cloudflareDNSResource
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

func TestHandleUpdateCloudflareDNSSettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithCloudflareDNSSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true,"token":"tok"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateCloudflareDNSSettings_EnableWithoutTokenRejected(t *testing.T) {
	s := newFakeCloudflareDNSSecrets()
	rt, db := newTestRouterWithCloudflareDNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateCloudflareDNSSettings_Success(t *testing.T) {
	s := newFakeCloudflareDNSSecrets()
	rt, db := newTestRouterWithCloudflareDNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"token":"shh"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetCloudflareDNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareDNSSettings() error = %v", err)
	}
	if !settings.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if s.values[store.CloudflareDNSTokenEnvKey] != "shh" {
		t.Errorf("stored token = %q, want shh", s.values[store.CloudflareDNSTokenEnvKey])
	}
}

func TestHandleUpdateCloudflareDNSSettings_OmittedTokenLeavesExistingIntact(t *testing.T) {
	s := newFakeCloudflareDNSSecrets()
	rt, db := newTestRouterWithCloudflareDNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	first := `{"enabled":true,"token":"original"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", first))

	second := `{"enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", second))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if s.values[store.CloudflareDNSTokenEnvKey] != "original" {
		t.Errorf("stored token = %q, want it left untouched at 'original'", s.values[store.CloudflareDNSTokenEnvKey])
	}
}

func TestHandleDisconnectCloudflareDNS_ClearsTokenAndDisables(t *testing.T) {
	s := newFakeCloudflareDNSSecrets()
	rt, db := newTestRouterWithCloudflareDNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", `{"enabled":true,"token":"tok"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/cloudflare-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	settings, err := db.GetCloudflareDNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareDNSSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Errorf("Enabled = true, want false after disconnect")
	}
	if _, ok := s.values[store.CloudflareDNSTokenEnvKey]; ok {
		t.Errorf("token still present after disconnect")
	}
}

func TestHandleDisconnectCloudflareDNS_Idempotent(t *testing.T) {
	s := newFakeCloudflareDNSSecrets()
	rt, db := newTestRouterWithCloudflareDNSSecrets(t, s)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/cloudflare-dns", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-disconnected setting, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestHandleUpdateCloudflareDNSSettings_RealSecretsManager_RoundTripsThroughEncryption
// mirrors the same whole-chain proof for the Tunnel token, against the
// distinct CloudflareDNSSecretsKey() namespace.
func TestHandleUpdateCloudflareDNSSettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithCloudflareDNSSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const token = "correct-horse-battery-staple-dns" //nolint:gosec // fake fixture, not a real credential
	body := `{"enabled":true,"token":"` + token + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.CloudflareDNSSecretsKey()

	gotToken, err := secretsManager.Resolve(ctx, key, store.CloudflareDNSTokenEnvKey)
	if err != nil {
		t.Fatalf("Resolve(token) error = %v", err)
	}
	if gotToken != token {
		t.Errorf("Resolve(token) = %q, want %q", gotToken, token)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, store.CloudflareDNSTokenEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue(token) error = %v", err)
	}
	if strings.Contains(string(ciphertext), token) {
		t.Error("token ciphertext contains the raw plaintext: encryption is not actually happening")
	}
}

func TestCloudflareDNSRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/settings/cloudflare-dns"},
		{http.MethodPut, "/api/v1/settings/cloudflare-dns"},
		{http.MethodDelete, "/api/v1/settings/cloudflare-dns"},
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
