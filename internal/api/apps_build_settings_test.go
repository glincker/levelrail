package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppBuildSettingsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/build-settings"},
		{http.MethodPut, "/api/v1/apps/web/build-settings"},
	})
}

func TestAppBuildSettingsSaveAndValidate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	if rec := ctxDo(t, rt, http.MethodGet, "/api/v1/apps/missing/build-settings", "", cookie); rec.Code != http.StatusNotFound {
		t.Fatalf("missing app GET = %d, want 404", rec.Code)
	}
	bad := []string{
		`{"target":"a b"}`, `{"target":"$(id)"}`, `{"provider":"rust"}`,
		`{"dockerfile_path":"../Dockerfile"}`, `{"builder":"dockerfile"}`,
	}
	for _, body := range bad {
		rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/build-settings", body, cookie)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
			t.Fatalf("PUT %s = %d, want 400 or 409 (%s)", body, rec.Code, rec.Body.String())
		}
	}
	rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/build-settings", `{"target":"runtime","no_cache":true,"provider":"node"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d (%s)", rec.Code, rec.Body.String())
	}
	var out appBuildSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Target != "runtime" || !out.NoCache || out.Provider != "node" {
		t.Fatalf("out = %+v err = %v", out, err)
	}
	stored, found, err := db.GetAppBuildSettings(context.Background(), "web")
	if err != nil || !found || stored.Target != "runtime" {
		t.Fatalf("stored = %+v found=%v err=%v", stored, found, err)
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err == nil {
		var seen bool
		for _, e := range entries {
			if e.Action == AuditActionAppBuildSettingsChange {
				seen = true
			}
		}
		if !seen {
			t.Fatal("no build settings audit row")
		}
	}
}

func TestAppBuildSettingsSourceFieldsNeedWriteSensitive(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	code, tok := mintToken(t, rt, cookie, `{"name":"bs-write","abilities":["read","write"]}`)
	if code != http.StatusCreated {
		t.Fatalf("mint token = %d", code)
	}
	put := func(body string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/build-settings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for _, body := range []string{`{"builder":"railpack"}`, `{"dockerfile_path":"Dockerfile"}`} {
		if got := put(body); got != http.StatusForbidden {
			t.Fatalf("write-only PUT %s = %d, want 403", body, got)
		}
	}
	if got := put(`{"target":"runtime"}`); got != http.StatusOK {
		t.Fatalf("write-only target PUT = %d, want 200", got)
	}
	if got := put(`{"target":"` + strings.Repeat("a", 2000) + `"}`); got != http.StatusBadRequest {
		t.Fatalf("over cap PUT = %d, want 400", got)
	}
}

func TestAppBuildSettingsHeldSnapshotWins(t *testing.T) {
	rt, db := newTestRouter(t)
	seedWebAppForTest(t, db)
	ctx := context.Background()
	if err := db.SaveAppBuildSettings(ctx, store.AppBuildSettings{AppName: "web", Target: "approved"}); err != nil {
		t.Fatal(err)
	}
	snap := rt.snapshotBuildSettings(ctx, "web")
	if err := db.SaveAppBuildSettings(ctx, store.AppBuildSettings{AppName: "web", Target: "changed", NoCache: true}); err != nil {
		t.Fatal(err)
	}
	var req deploy.Request
	rt.applyAppBuildSettings(replayContext(ctx, deploy.HeldRequest{BuildSettings: snap}), "web", &req)
	if req.Target != "approved" || req.NoCache {
		t.Fatalf("replay used live settings: %+v", req)
	}
	var live deploy.Request
	rt.applyAppBuildSettings(replayContext(ctx, deploy.HeldRequest{}), "web", &live)
	if live.Target != "changed" {
		t.Fatalf("queued replay target = %q, want changed", live.Target)
	}
	if empty := rt.snapshotBuildSettings(ctx, "other"); empty == nil {
		t.Fatal("snapshot must never be nil")
	}
}

func TestAppBuildSettingsReachDeployRequest(t *testing.T) {
	req := deploy.Request{}
	applyStoredBuildSettings(store.AppBuildSettings{Target: "runtime", NoCache: true, Provider: "node"}, &req)
	if req.Target != "runtime" || !req.NoCache || req.RailpackProvider != "node" {
		t.Fatalf("req = %+v", req)
	}
}
