package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSecretKeyInfo struct {
	key    string
	locked bool
}

// fakeSecretSetter is a hand-written fake for SecretSetter, the same
// pattern every other test in this package uses instead of a mocking
// framework.
type fakeSecretSetter struct {
	err                error
	calls              int
	lastService        string
	lastKey, lastValue string
	locked             bool
	keys               []fakeSecretKeyInfo
	listErr            error
	setLockedErr       error
	lastLockedKey      string
	lastLockedTo       bool
}

func (f *fakeSecretSetter) SetValueGuarded(_ context.Context, serviceName, envKey, plaintext string, overwriteLocked bool) error {
	f.calls++
	f.lastService = serviceName
	f.lastKey = envKey
	f.lastValue = plaintext
	if f.locked && !overwriteLocked {
		return secrets.ErrSecretLocked
	}
	return f.err
}

func (f *fakeSecretSetter) ListKeys(_ context.Context, _ string) ([]store.SecretKeyInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]store.SecretKeyInfo, len(f.keys))
	for i, k := range f.keys {
		out[i] = store.SecretKeyInfo{Key: k.key, Locked: k.locked}
	}
	return out, nil
}

func (f *fakeSecretSetter) SetLocked(_ context.Context, _, envKey string, locked bool) error {
	if f.setLockedErr != nil {
		return f.setLockedErr
	}
	f.lastLockedKey = envKey
	f.lastLockedTo = locked
	return nil
}

func newTestRouterWithSecrets(t *testing.T, secrets SecretSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithSecretSetter(secrets)), db
}

func TestHandleSetSecret_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithSecretSetter
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleSetSecret_Success(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty: a secret value must never be echoed back", rec.Body.String())
	}
	if setter.calls != 1 || setter.lastService != "web" || setter.lastKey != "API_KEY" || setter.lastValue != "sk-abc" {
		t.Errorf("setter called with (%q, %q, %q), calls=%d, want (web, API_KEY, sk-abc), calls=1",
			setter.lastService, setter.lastKey, setter.lastValue, setter.calls)
	}
}

func TestHandleSetSecret_AppNotFound(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if setter.calls != 0 {
		t.Errorf("setter.calls = %d, want 0: must not set a secret for an app that doesn't exist", setter.calls)
	}
}

func TestHandleSetSecret_EmptyValueRejected(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if setter.calls != 0 {
		t.Errorf("setter.calls = %d, want 0", setter.calls)
	}
}

func TestHandleSetSecret_MalformedBody(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetSecret_RequiresAuth(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, _ := newTestRouterWithSecrets(t, setter)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if setter.calls != 0 {
		t.Errorf("setter.calls = %d, want 0", setter.calls)
	}
}

func TestHandleSetSecret_PlainWriteTokenForbidden(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	const plaintext = "write-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "app-editor", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", strings.NewReader(`{"value":"sk-abc"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: a plain write-scoped token must not be able to write secret values", rec.Code, http.StatusForbidden)
	}
	if setter.calls != 0 {
		t.Errorf("setter.calls = %d, want 0", setter.calls)
	}
}

func TestHandleSetSecret_WriteSensitiveTokenSucceeds(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	const plaintext = "write-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_sensitive", Name: "secrets-editor", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWriteSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", strings.NewReader(`{"value":"sk-abc"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if setter.calls != 1 {
		t.Errorf("setter.calls = %d, want 1", setter.calls)
	}
}

func TestHandleSetSecret_StoreErrorPropagates(t *testing.T) {
	setter := &fakeSecretSetter{err: errors.New("master key not configured")}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleSetSecret_LockedWithoutOverwriteFlag_Returns409(t *testing.T) {
	setter := &fakeSecretSetter{locked: true}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-new"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleSetSecret_LockedWithOverwriteFlag_Succeeds(t *testing.T) {
	setter := &fakeSecretSetter{locked: true}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/secrets/API_KEY", `{"value":"sk-new","overwrite_locked":true}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleListSecrets_Success(t *testing.T) {
	setter := &fakeSecretSetter{keys: []fakeSecretKeyInfo{{key: "API_KEY", locked: false}, {key: "DB_PASSWORD", locked: true}}}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/secrets", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"key":"API_KEY"`) || !strings.Contains(rec.Body.String(), `"locked":true`) {
		t.Errorf("body = %s, want it to contain both keys with their locked state", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-") {
		t.Errorf("body = %s, must never contain a value", rec.Body.String())
	}
}

func TestHandleListSecrets_AppNotFound(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/secrets", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetSecretLock_Success(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/secrets/API_KEY/lock", `{"locked":true}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if setter.lastLockedKey != "API_KEY" || !setter.lastLockedTo {
		t.Errorf("locked key = %q, to = %v, want API_KEY, true", setter.lastLockedKey, setter.lastLockedTo)
	}
}

func TestHandleSetSecretLock_KeyNotFound(t *testing.T) {
	setter := &fakeSecretSetter{setLockedErr: store.ErrSecretValueNotFound}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/secrets/GHOST/lock", `{"locked":true}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetSecretLock_PlainWriteTokenForbidden(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	const plaintext = "write-only-token-lock" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_lock", Name: "app-editor", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/secrets/API_KEY/lock", strings.NewReader(`{"locked":true}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: a plain write-scoped token must not be able to lock/unlock a secret", rec.Code, http.StatusForbidden)
	}
}

func TestHandleListSecrets_PlainReadTokenSucceeds(t *testing.T) {
	setter := &fakeSecretSetter{keys: []fakeSecretKeyInfo{{key: "API_KEY", locked: true}}}
	rt, db := newTestRouterWithSecrets(t, setter)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	const plaintext = "read-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read", Name: "viewer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/secrets", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: a key name is no more sensitive than any other read-scoped app resource", rec.Code, http.StatusOK)
	}
}
