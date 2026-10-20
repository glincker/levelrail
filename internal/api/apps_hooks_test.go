package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppHookRunsRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/hook-runs", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetAppHookRuns_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/hook-runs", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetAppHookRuns_NoneRecorded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/hook-runs", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got appHookRunsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PreDeploy != nil || got.PostDeploy != nil {
		t.Errorf("got = %+v, want both nil (no hook has run yet)", got)
	}
}

func TestHandleGetAppHookRuns_BothRecorded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	ranAt := time.Now().UTC()
	if err := db.UpsertHookRun(ctx, store.HookRun{ServiceName: "web", HookType: store.HookTypePreDeploy, Command: "migrate", ExitCode: 0, Success: true, Output: "ok", RanAt: ranAt}); err != nil {
		t.Fatalf("UpsertHookRun(pre) error = %v", err)
	}
	if err := db.UpsertHookRun(ctx, store.HookRun{ServiceName: "web", HookType: store.HookTypePostDeploy, Command: "notify", ExitCode: 1, Success: false, Output: "failed", RanAt: ranAt}); err != nil {
		t.Fatalf("UpsertHookRun(post) error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/hook-runs", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got appHookRunsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PreDeploy == nil || got.PreDeploy.Command != "migrate" || !got.PreDeploy.Success {
		t.Errorf("PreDeploy = %+v, want a successful migrate run", got.PreDeploy)
	}
	if got.PostDeploy == nil || got.PostDeploy.Command != "notify" || got.PostDeploy.Success || got.PostDeploy.ExitCode != 1 {
		t.Errorf("PostDeploy = %+v, want a failed notify run with exit code 1", got.PostDeploy)
	}
}

// TestAppResource_Hooks_RoundTripsThroughUpdate asserts appResource.Hooks
// (apps.go) is a settable field, not response-only: a PUT that adds
// hooks must be readable back on the very next GET.
func TestAppResource_Hooks_RoundTripsThroughUpdate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	body := `{"name":"web","image":"img:v1","port":8080,"strategy":"blue-green","replicas":1,"hooks":{"pre_deploy":"migrate"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Hooks == nil || got.Hooks.PreDeploy != "migrate" {
		t.Errorf("Hooks = %+v, want PreDeploy=migrate", got.Hooks)
	}
}

// TestAppResource_Hooks_BothEmpty_Rejected covers validateAppResource's
// own new check: a non-nil hooks block with neither command set can only
// be a caller mistake.
func TestAppResource_Hooks_BothEmpty_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	body := `{"name":"web","image":"img:v1","port":8080,"strategy":"blue-green","replicas":1,"hooks":{}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
