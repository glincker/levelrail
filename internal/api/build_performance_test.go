package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBuildStatsAndLimits(t *testing.T) {
	t.Setenv(build.EnvBuildTimeout, "10m")
	t.Setenv(build.EnvBuildMaxConcurrent, "3")
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithBuildPerformance(db))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	rec := BuildRunRecorder(db)
	for i, d := range []time.Duration{10 * time.Second, 20 * time.Second, 30 * time.Second} {
		if err := rec.RecordBuildRun(ctx, build.Run{App: "web", Tag: "web:x", StartedAt: time.Unix(int64(i), 0), Duration: d,
			Status: build.RunSuccess, CachedSteps: 1, TotalSteps: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.RecordBuildRun(ctx, build.Run{App: "web", Tag: "web:y", StartedAt: time.Unix(9, 0), Status: build.RunTimeout}); err != nil {
		t.Fatal(err)
	}

	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, authedRequest(t, cookie, method, path, body))
		return w
	}

	w := do(http.MethodGet, "/api/v1/apps/web/build-stats?limit=10", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats %d: %s", w.Code, w.Body)
	}
	var stats buildStatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Trend.Builds != 4 || stats.Trend.Succeeded != 3 || stats.Trend.TimedOut != 1 || stats.Trend.P50Seconds != 20 ||
		!stats.Trend.CacheKnown || stats.Trend.CacheHitRatio != 0.5 || stats.Runs[0].Status != "timeout" {
		t.Fatalf("stats = %+v", stats)
	}

	if w = do(http.MethodGet, "/api/v1/apps/ghost/build-stats", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown app: %d", w.Code)
	}
	if w = do(http.MethodGet, "/api/v1/apps/web/build-stats?limit=0", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: %d", w.Code)
	}

	var lim buildLimitsResponse
	_ = json.Unmarshal(do(http.MethodGet, "/api/v1/build-limits", "").Body.Bytes(), &lim)
	if lim.Effective.MaxConcurrent != 3 || lim.Effective.TimeoutSeconds != 600 || lim.Overrides.MaxConcurrent != nil {
		t.Fatalf("defaults = %+v", lim)
	}
	if w = do(http.MethodPut, "/api/v1/build-limits", `{"timeout_seconds":90}`); w.Code != http.StatusOK {
		t.Fatalf("put %d: %s", w.Code, w.Body)
	}
	_ = json.Unmarshal(do(http.MethodGet, "/api/v1/build-limits", "").Body.Bytes(), &lim)
	if lim.Effective.TimeoutSeconds != 90 || lim.Effective.MaxConcurrent != 3 || lim.Default.TimeoutSeconds != 600 {
		t.Fatalf("after put = %+v", lim)
	}
	if got := BuildLimitsSource(db, build.LimitsFromEnv(func(string) (string, bool) { return "", false }))(ctx, "web"); got.Timeout != 90*time.Second {
		t.Fatalf("source = %+v", got)
	}
	for _, bad := range []string{`{}`, `{"max_concurrent":-1}`, `{"timeout_seconds":99999999}`} {
		if w = do(http.MethodPut, "/api/v1/build-limits", bad); w.Code != http.StatusBadRequest {
			t.Fatalf("put %s: %d", bad, w.Code)
		}
	}
	lim = buildLimitsResponse{}
	_ = json.Unmarshal(do(http.MethodDelete, "/api/v1/build-limits", "").Body.Bytes(), &lim)
	if lim.Overrides.TimeoutSeconds != nil || lim.Effective.TimeoutSeconds != 600 {
		t.Fatalf("after reset = %+v", lim)
	}
}

func TestAppBuildLimitsOverrideGlobal(t *testing.T) {
	t.Setenv(build.EnvBuildMemoryMB, "4096")
	t.Setenv(build.EnvBuildCPUMillis, "2000")
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithBuildPerformance(db))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, n := range []string{"web", "api"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "levelrail/" + n + ":1", Port: 3000}); err != nil {
			t.Fatal(err)
		}
	}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, authedRequest(t, cookie, method, path, body))
		return w
	}

	if w := do(http.MethodPut, "/api/v1/apps/web/build-limits", `{"memory_mb":1024,"max_concurrent":9}`); w.Code != http.StatusOK {
		t.Fatalf("put %d: %s", w.Code, w.Body)
	}
	var got appBuildLimitsResponse
	_ = json.Unmarshal(do(http.MethodGet, "/api/v1/apps/web/build-limits", "").Body.Bytes(), &got)
	if got.Effective.MemoryMB != 1024 || got.Effective.CPUMillis != 2000 || got.Global.MemoryMB != 4096 ||
		got.Overrides.MemoryMB == nil || got.Overrides.MaxConcurrent != nil {
		t.Fatalf("web = %+v", got)
	}

	src := BuildLimitsSource(db, build.LimitsFromEnv(os.LookupEnv))
	if l := src(ctx, "web"); l.MemoryMB != 1024 || l.CPUMillis != 2000 {
		t.Fatalf("web source = %+v", l)
	}
	if l := src(ctx, "api"); l.MemoryMB != 4096 {
		t.Fatalf("api must keep the global default, got %+v", l)
	}

	for _, bad := range []string{`{}`, `{"memory_mb":-1}`, `{"cpu_millis":99999999}`} {
		if w := do(http.MethodPut, "/api/v1/apps/web/build-limits", bad); w.Code != http.StatusBadRequest {
			t.Fatalf("put %s: %d", bad, w.Code)
		}
	}
	if w := do(http.MethodPut, "/api/v1/apps/ghost/build-limits", `{"memory_mb":1}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown app: %d", w.Code)
	}

	got = appBuildLimitsResponse{}
	_ = json.Unmarshal(do(http.MethodDelete, "/api/v1/apps/web/build-limits", "").Body.Bytes(), &got)
	if got.Overrides.MemoryMB != nil || got.Effective.MemoryMB != 4096 {
		t.Fatalf("after reset = %+v", got)
	}
}
