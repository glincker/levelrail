package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newRotationRouter(t *testing.T) (*Router, *store.DB, *secrets.Manager, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	mgr := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(mgr))
	rt.SetSecretRotation(db, 5, 0)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := mgr.SetValue(context.Background(), "web", "JWT_SECRET", "original-value"); err != nil {
		t.Fatal(err)
	}
	return rt, db, mgr, cookie
}

func rotationDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestSecretRotationPolicyLifecycle(t *testing.T) {
	rt, _, mgr, cookie := newRotationRouter(t)
	ctx := context.Background()
	base := "/api/v1/apps/web/secrets/JWT_SECRET/rotation"

	bad := []string{
		`{"interval_days":0}`,
		`{"interval_days":30,"grace_hours":-1}`,
		`{"interval_days":1,"grace_hours":48}`,
	}
	for _, body := range bad {
		if rec := rotationDo(t, rt, cookie, http.MethodPut, base, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", body, rec.Code)
		}
	}
	if rec := rotationDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/NOPE/rotation", `{"interval_days":30}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing secret = %d, want 404", rec.Code)
	}
	if rec := rotationDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/JWT_SECRET_PREVIOUS/rotation", `{"interval_days":30}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("grace key = %d, want 400", rec.Code)
	}

	rec := rotationDo(t, rt, cookie, http.MethodPut, base, `{"interval_days":30,"grace_hours":12,"owner":"platform","auto_rotate":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d: %s", rec.Code, rec.Body.String())
	}
	rec = rotationDo(t, rt, cookie, http.MethodPost, base+"/rotate", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("rotate non-auto = %d, want 409", rec.Code)
	}
	if v, _ := mgr.Resolve(ctx, "web", "JWT_SECRET"); v != "original-value" {
		t.Fatal("non-auto policy must never change the value")
	}

	if rec := rotationDo(t, rt, cookie, http.MethodPut, base, `{"interval_days":30,"grace_hours":12,"owner":"platform","auto_rotate":true}`); rec.Code != http.StatusOK {
		t.Fatalf("put auto = %d", rec.Code)
	}
	rec = rotationDo(t, rt, cookie, http.MethodPost, base+"/rotate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate = %d: %s", rec.Code, rec.Body.String())
	}
	newValue, err := mgr.Resolve(ctx, "web", "JWT_SECRET")
	if err != nil || newValue == "original-value" {
		t.Fatalf("value not rotated: %v", err)
	}
	if strings.Contains(rec.Body.String(), newValue) || strings.Contains(rec.Body.String(), "original-value") {
		t.Fatal("response leaked a secret value")
	}
	if prev, _ := mgr.Resolve(ctx, "web", "JWT_SECRET_PREVIOUS"); prev != "original-value" {
		t.Fatal("previous value not kept for the grace window")
	}

	rec = rotationDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/secret-rotation", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"owner":"platform"`) || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := rotationDo(t, rt, cookie, http.MethodDelete, base, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
}

func TestSecretRotationOverdueRaisesAttention(t *testing.T) {
	rt, db, _, cookie := newRotationRouter(t)
	ctx := context.Background()
	if err := db.UpsertSecretRotationPolicy(ctx, store.SecretRotationPolicy{ServiceName: "web", EnvKey: "JWT_SECRET", IntervalDays: 1, Owner: "ops"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE service_secret_values SET updated_at = '2020-01-01T00:00:00.000Z' WHERE env_key = 'JWT_SECRET'`); err != nil {
		t.Fatal(err)
	}
	rec := rotationDo(t, rt, cookie, http.MethodGet, "/api/v1/attention/feed", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "secret_rotation_due") || !strings.Contains(rec.Body.String(), "web/JWT_SECRET") {
		t.Fatalf("feed = %d %s", rec.Code, rec.Body.String())
	}
	rec = rotationDo(t, rt, cookie, http.MethodGet, "/api/v1/secret-rotation", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"overdue"`) {
		t.Fatalf("overview = %d %s", rec.Code, rec.Body.String())
	}
}
