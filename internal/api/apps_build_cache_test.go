package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeAppCacher struct {
	usage  build.AppCacheUsage
	res    build.AppCachePruneResult
	pruned []string
}

func (f *fakeAppCacher) AppCacheUsage(_ context.Context, _ string, _ []string) (build.AppCacheUsage, error) {
	return f.usage, nil
}

func (f *fakeAppCacher) PruneAppCache(_ context.Context, id string, _ []string) (build.AppCachePruneResult, error) {
	f.pruned = append(f.pruned, id)
	return f.res, nil
}

func TestAppCacheMemoExpiresAndDrops(t *testing.T) {
	m := &appCacheMemo{entries: map[string]appCacheMemoEntry{}}
	m.put("web", appBuildCacheResource{CacheID: "web", SizeBytes: 5})
	if r, ok := m.get("web"); !ok || r.SizeBytes != 5 {
		t.Fatal("fresh entry should hit")
	}
	m.drop("web")
	if _, ok := m.get("web"); ok {
		t.Fatal("dropped entry should miss")
	}
	m.entries["old"] = appCacheMemoEntry{at: time.Now().Add(-time.Hour)}
	if _, ok := m.get("old"); ok {
		t.Fatal("stale entry should miss")
	}
}

func TestAppBuildCacheShowAndClear(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	fake := &fakeAppCacher{usage: build.AppCacheUsage{Bytes: 412, Records: 3}, res: build.AppCachePruneResult{FreedBytes: 412, Removed: 3}}
	WithAppBuildCache(fake)(rt)
	path := "/api/v1/apps/web/build-cache"

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, path, ""))
	var got appBuildCacheResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.SizeBytes != 412 || got.CacheID != "web" {
		t.Fatalf("show = %d %s", rec.Code, rec.Body.String())
	}

	attempt := store.DeployAttempt{ID: "att1", ServiceName: "web", Image: "web:1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}
	if err := db.SaveDeployAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, path, "")); rec.Code != http.StatusConflict || len(fake.pruned) != 0 {
		t.Fatalf("running build must give 409 without pruning: %d", rec.Code)
	}
	if err := db.FinishDeployAttempt(context.Background(), "att1", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	rec = serve(rt, authedRequest(t, cookie, http.MethodDelete, path, ""))
	var cleared appBuildCacheClearResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &cleared) != nil || cleared.FreedBytes != 412 || cleared.RemovedCount != 3 {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.pruned) != 1 || fake.pruned[0] != "web" {
		t.Fatalf("pruned %v", fake.pruned)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/nope/build-cache", "")); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}
