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

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeAIAssistantSecrets is a hand-written fake for AIAssistantSecrets,
// the same pattern fakeEmailSecretsStore already establishes for the
// structurally identical email-settings interface.
type fakeAIAssistantSecrets struct {
	err    error
	values map[string]string
}

func newFakeAIAssistantSecrets() *fakeAIAssistantSecrets {
	return &fakeAIAssistantSecrets{values: map[string]string{}}
}

func (f *fakeAIAssistantSecrets) SetValue(_ context.Context, _, envKey, plaintext string) error {
	if f.err != nil {
		return f.err
	}
	f.values[envKey] = plaintext
	return nil
}

func (f *fakeAIAssistantSecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeAIAssistantSecrets) DeleteAll(_ context.Context, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.values = map[string]string{}
	return nil
}

func newTestRouterWithAIAssistantSecrets(t *testing.T, secrets AIAssistantSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithAIAssistantSecrets(secrets)), db
}

func TestHandleGetAIAssistantSettings_SeededDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ai-assistant", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got aiAssistantSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Configured || got.Provider != "" || got.Model != "" {
		t.Errorf("got %+v, want all-empty default", got)
	}
}

func TestHandleUpdateAIAssistantSettings_NoSecretsConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithAIAssistantSecrets
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"anthropic","model":"claude-sonnet-5","api_key":"sk-ant-test"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleUpdateAIAssistantSettings_Success(t *testing.T) {
	fakeSecrets := newFakeAIAssistantSecrets()
	rt, db := newTestRouterWithAIAssistantSecrets(t, fakeSecrets)
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"anthropic","model":"claude-sonnet-5","api_key":"sk-ant-test"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-ant-test") {
		t.Errorf("response contains the raw api key: %s", rec.Body.String())
	}

	var got aiAssistantSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Configured || got.Provider != "anthropic" || got.Model != "claude-sonnet-5" {
		t.Errorf("got %+v, want configured=true provider=anthropic model=claude-sonnet-5", got)
	}
	if fakeSecrets.values[ai.SecretsAPIKeyEnvKey] != "sk-ant-test" {
		t.Errorf("stored api key = %q, want sk-ant-test", fakeSecrets.values[ai.SecretsAPIKeyEnvKey])
	}

	settings, err := db.GetAIAssistantSettings(context.Background())
	if err != nil {
		t.Fatalf("GetAIAssistantSettings() error = %v", err)
	}
	if settings.Provider != "anthropic" || settings.Model != "claude-sonnet-5" {
		t.Errorf("stored settings = %+v, want anthropic/claude-sonnet-5", settings)
	}
}

func TestHandleUpdateAIAssistantSettings_RejectsUnknownProvider(t *testing.T) {
	rt, db := newTestRouterWithAIAssistantSecrets(t, newFakeAIAssistantSecrets())
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"openai","model":"gpt-5","api_key":"sk-test"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleUpdateAIAssistantSettings_RequiresModelAndKey(t *testing.T) {
	rt, db := newTestRouterWithAIAssistantSecrets(t, newFakeAIAssistantSecrets())
	cookie := loginTestSession(t, rt, db)

	tests := []string{
		`{"provider":"anthropic","api_key":"sk-test"}`,
		`{"provider":"anthropic","model":"claude-sonnet-5"}`,
	}
	for _, body := range tests {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleDeleteAIAssistantSettings_ClearsConfiguration(t *testing.T) {
	fakeSecrets := newFakeAIAssistantSecrets()
	rt, db := newTestRouterWithAIAssistantSecrets(t, fakeSecrets)
	cookie := loginTestSession(t, rt, db)

	body := `{"provider":"anthropic","model":"claude-sonnet-5","api_key":"sk-ant-test"}`
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/settings/ai-assistant", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got aiAssistantSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Configured {
		t.Errorf("got %+v, want configured=false after delete", got)
	}
	if len(fakeSecrets.values) != 0 {
		t.Errorf("fakeSecrets.values = %v, want empty after delete", fakeSecrets.values)
	}
}

// TestHandleUpdateAIAssistantSettings_RealSecretsManager_RoundTripsThroughEncryption
// is the whole-chain proof (a real master key and secrets.Manager, not a
// fake) that the key this handler writes actually round-trips through
// envelope encryption, and that only ciphertext reaches disk, the same
// proof TestHandleUpdateEmailSettings_RealSecretsManager_RoundTripsThroughEncryption
// already establishes for email settings.
func TestHandleUpdateAIAssistantSettings_RealSecretsManager_RoundTripsThroughEncryption(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithAIAssistantSecrets(secretsManager))
	cookie := loginTestSession(t, rt, db)

	const apiKey = "sk-ant-correct-horse-battery-staple" //nolint:gosec // fake fixture, not a real credential
	body := `{"provider":"anthropic","model":"claude-sonnet-5","api_key":"` + apiKey + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ai-assistant", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	ctx := context.Background()
	key := store.AIAssistantSecretsKey()
	got, err := secretsManager.Resolve(ctx, key, ai.SecretsAPIKeyEnvKey)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != apiKey {
		t.Errorf("Resolve() = %q, want %q", got, apiKey)
	}

	ciphertext, err := db.GetSecretValue(ctx, key, ai.SecretsAPIKeyEnvKey)
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if strings.Contains(string(ciphertext), apiKey) {
		t.Error("ciphertext contains the raw api key: encryption is not actually happening")
	}
}

func TestAIAssistantSettingsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []routeCase{
		{http.MethodGet, "/api/v1/settings/ai-assistant"},
		{http.MethodPut, "/api/v1/settings/ai-assistant"},
		{http.MethodDelete, "/api/v1/settings/ai-assistant"},
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

func TestHandleUpdateAIAssistantSettings_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithAIAssistantSecrets(t, newFakeAIAssistantSecrets())
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_ai", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"provider":"anthropic","model":"claude-sonnet-5","api_key":"sk-test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ai-assistant", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the ai-assistant settings write", rec.Code, http.StatusForbidden)
	}
}
