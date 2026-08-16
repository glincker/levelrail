package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// linkServiceToApp is this file's own copy of internal/store's
// test-only helper of the same name: apps_group_test.go is a different
// package (api, not store), so it can't reach that unexported test
// helper directly.
func linkServiceToApp(t *testing.T, db *store.DB, serviceName, appID string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `UPDATE desired_services SET app_id = ? WHERE name = ?`, appID, serviceName); err != nil {
		t.Fatalf("link service %q to app %q: %v", serviceName, appID, err)
	}
}

func TestAppGroupRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/group", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// getAppGroup issues an authenticated GET against path and decodes a
// 200 response as appGroupResource, failing the test on any other
// status or a malformed body.
func getAppGroup(t *testing.T, rt *Router, cookie *http.Cookie, path string) appGroupResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got appGroupResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestHandleGetAppGroup_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/group", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleGetAppGroup_ServiceWithNoApp covers a service saved with no
// app_id (every service saved through this stage's write path, since
// SaveDesiredService never sets it): the group endpoint must treat it
// as its own one-service group, not 404 or error.
func TestHandleGetAppGroup_ServiceWithNoApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "solo", Image: "levelrail/solo:1", Port: 3000}); err != nil {
		t.Fatalf("seed SaveDesiredService: %v", err)
	}

	got := getAppGroup(t, rt, cookie, "/api/v1/apps/solo/group")
	if got.AppID != "" {
		t.Errorf("AppID = %q, want empty", got.AppID)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "solo" {
		t.Fatalf("Services = %+v, want one service named solo", got.Services)
	}
	if got.Status.Label != "No status yet" || got.Status.Variant != "muted" {
		t.Errorf("Status = %+v, want No status yet/muted", got.Status)
	}
}

// TestHandleGetAppGroup_MultiServiceApp is stage 1's core read-path
// scenario: two services under one app, both returned together, with a
// worst-condition-wins rollup status (one True, one False -> the group
// as a whole reads "Attention needed", the same rule
// summarizeAppConditions already applies within a single app's own
// conditions, now applied across an app's services).
func TestHandleGetAppGroup_MultiServiceApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	app := store.App{ID: "app_demo", Name: "demo", CreatedAt: "2026-08-14T00:00:00Z", UpdatedAt: "2026-08-14T00:00:00Z"}
	if err := db.SaveApp(ctx, app); err != nil {
		t.Fatalf("SaveApp: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "worker", Image: "levelrail/worker:1", Port: 4000}); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	linkServiceToApp(t, db, "web", app.ID)
	linkServiceToApp(t, db, "worker", app.ID)

	if err := db.UpsertConditions(ctx, "application/web", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
	}); err != nil {
		t.Fatalf("upsert web conditions: %v", err)
	}
	if err := db.UpsertConditions(ctx, "application/worker", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "CrashLoop"},
	}); err != nil {
		t.Fatalf("upsert worker conditions: %v", err)
	}

	got := getAppGroup(t, rt, cookie, "/api/v1/apps/web/group")
	if got.AppID != app.ID {
		t.Errorf("AppID = %q, want %q", got.AppID, app.ID)
	}
	if len(got.Services) != 2 || got.Services[0].Name != "web" || got.Services[1].Name != "worker" {
		t.Fatalf("Services = %+v, want [web, worker]", got.Services)
	}
	for _, s := range got.Services {
		if s.AppID != app.ID {
			t.Errorf("service %q app_id = %q, want %q", s.Name, s.AppID, app.ID)
		}
	}
	if got.Status.Label != "Attention needed" || got.Status.Variant != "destructive" {
		t.Errorf("Status = %+v, want Attention needed/destructive (worst-condition-wins across both services)", got.Status)
	}
}
