package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRemoveExistingVolumesTeardownDBBypassesLock(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{volumes: []docker.NamedVolume{{Name: "db-pv-data"}}}
	rt, _ := newTestRouterWithOrphanedVolumeManager(t, fake)
	if failed := rt.removeExistingVolumes(context.Background(), []string{"db-pv-data"}, true); len(failed) != 0 {
		t.Fatalf("failed = %v, want none", failed)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "db-pv-data" {
		t.Fatalf("removed = %v, want db-pv-data", fake.removed)
	}
}

func TestRemoveExistingVolumesNonTeardownReportsLockedVolume(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{volumes: []docker.NamedVolume{{Name: "db-pv-data"}, {Name: "plain"}}}
	rt, _ := newTestRouterWithOrphanedVolumeManager(t, fake)
	failed := rt.removeExistingVolumes(context.Background(), []string{"db-pv-data", "plain"}, false)
	if len(failed) != 1 || !strings.Contains(failed[0], "db-pv-data") {
		t.Fatalf("failed = %v, want the locked volume reported", failed)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "plain" {
		t.Fatalf("removed = %v, want only plain", fake.removed)
	}
}

func TestRetryDatabaseTeardownSkipsRecreatedDatabase(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	fake := &flakyTeardownRuntime{fakeExecAppRuntime: &fakeExecAppRuntime{}}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }
	ctx := context.Background()

	if err := rt.recordDatabaseTeardown(ctx, "main", "", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.RetryDatabaseTeardowns(ctx); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if fake.removeCalls != 0 {
		t.Fatalf("remove calls = %d, a recreated database must not be torn down", fake.removeCalls)
	}
	if jobs, _ := db.ListDatabaseTeardownJobs(ctx); len(jobs) != 0 {
		t.Fatalf("superseded job must be cleared: %+v", jobs)
	}
}

func TestCreateDatabaseConflictsWhileTeardownPending(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDatabaseTeardownJob(context.Background(), "main", "", true); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"main","engine":"postgres","version":"16"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}
