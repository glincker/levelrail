package api

import (
	"context"
	"encoding/json"
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
	key       string
	locked    bool
	updatedAt time.Time
}

// fakeSecretSetter is a hand-written fake for SecretSetter, the same
// pattern every other test in this package uses instead of a mocking
// framework.
type fakeSecretSetterCall struct {
	service, key, value string
	overwriteLocked     bool
}

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
	// existsValues/existsErr back Exists: keyed by serviceName+"/"+envKey,
	// so a test can make one specific (service, key) pair "exist" without
	// affecting any other lookup the router makes during the same
	// request (e.g. databases_test.go's TLSEnabled coverage).
	existsValues map[string]bool
	existsErr    error
	// sets records every SetValueGuarded call in order, for tests that
	// need to verify more than just the most recent one (e.g. create-app
	// with multiple secrets at once). lastService/lastKey/lastValue above
	// stay the older, single-call surface every pre-existing test uses.
	sets []fakeSecretSetterCall
	// resolveValues/resolveErr back Resolve: keyed by serviceName+"/"+envKey,
	// the same shape existsValues above already uses. A missing key
	// returns secrets.ErrValueNotFound, matching *secrets.Manager's own
	// "no value set" behavior, not a generic error.
	resolveValues map[string]string
	resolveErr    error
}

func (f *fakeSecretSetter) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.existsValues[serviceName+"/"+envKey], nil
}

func (f *fakeSecretSetter) SetValueGuarded(_ context.Context, serviceName, envKey, plaintext string, overwriteLocked bool) error {
	f.calls++
	f.lastService = serviceName
	f.lastKey = envKey
	f.lastValue = plaintext
	f.sets = append(f.sets, fakeSecretSetterCall{service: serviceName, key: envKey, value: plaintext, overwriteLocked: overwriteLocked})
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
		out[i] = store.SecretKeyInfo{Key: k.key, Locked: k.locked, UpdatedAt: k.updatedAt}
	}
	return out, nil
}

func (f *fakeSecretSetter) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	v, ok := f.resolveValues[serviceName+"/"+envKey]
	if !ok {
		return "", secrets.ErrValueNotFound
	}
	return v, nil
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

// TestHandleListSecrets_StaleFlag proves GET /apps/{name}/secrets flags a
// key as stale once its UpdatedAt is older than the configured secret
// rotation warning threshold (WithSecretRotationWarnAge), and leaves a
// fresh key unflagged, both in the same response.
func TestHandleListSecrets_StaleFlag(t *testing.T) {
	setter := &fakeSecretSetter{keys: []fakeSecretKeyInfo{
		{key: "OLD_KEY", updatedAt: time.Now().Add(-48 * time.Hour)},
		{key: "FRESH_KEY", updatedAt: time.Now().Add(-time.Hour)},
	}}
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db,
		WithSecretSetter(setter),
		WithSecretRotationWarnAge(24*time.Hour),
	)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/secrets", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []secretKeyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v, body = %s", err, rec.Body.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	byKey := map[string]secretKeyResource{}
	for _, r := range got {
		byKey[r.Key] = r
	}
	if !byKey["OLD_KEY"].Stale {
		t.Errorf("OLD_KEY (48h old, 24h threshold) stale = false, want true")
	}
	if byKey["FRESH_KEY"].Stale {
		t.Errorf("FRESH_KEY (1h old, 24h threshold) stale = true, want false")
	}
	if byKey["OLD_KEY"].UpdatedAt == "" {
		t.Errorf("OLD_KEY updated_at is empty, want a populated timestamp")
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
