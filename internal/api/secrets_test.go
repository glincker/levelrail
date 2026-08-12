package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeSecretSetter is a hand-written fake for SecretSetter, the same
// pattern every other test in this package uses instead of a mocking
// framework.
type fakeSecretSetter struct {
	err                error
	calls              int
	lastService        string
	lastKey, lastValue string
}

func (f *fakeSecretSetter) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.calls++
	f.lastService = serviceName
	f.lastKey = envKey
	f.lastValue = plaintext
	return f.err
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
