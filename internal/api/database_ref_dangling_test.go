package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

// A connection records the database instance, and a force delete leaves a dangling marker a same-name database cannot satisfy.
func TestForceDeleteDatabaseInvalidatesRefs(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectByNameCalls: make(chan struct{}, 4)}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7"}`)
	ctx := context.Background()

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	first, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	for _, ref := range svc.DatabaseEnv {
		if ref.Instance != first.InstanceID || ref.Instance == "" {
			t.Fatalf("ref instance = %q, want %q", ref.Instance, first.InstanceID)
		}
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main", ""))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "web") {
		t.Fatalf("in-use status = %d, body = %s, want 409 naming the visible app", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main?force=true", ""))
	if rec.Code != http.StatusNoContent || rec.Header().Get(dangledAppsHeader) != "web" {
		t.Fatalf("force status = %d, header = %q, want 204 and web", rec.Code, rec.Header().Get(dangledAppsHeader))
	}

	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7"}`)
	svc, err = db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if len(svc.DatabaseEnv) == 0 {
		t.Fatal("refs were removed, want them kept as dangling")
	}
	for _, ref := range svc.DatabaseEnv {
		if ref.Instance != store.DanglingDatabaseInstance {
			t.Fatalf("ref instance = %q, want dangling", ref.Instance)
		}
	}
}
