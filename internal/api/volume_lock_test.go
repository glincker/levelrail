package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestVolumeLockCleanupSkipsDatabaseVolumes(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{volumes: []docker.NamedVolume{
		{Name: "db-gone-data", SizeBytes: 10},
		{Name: "plain-orphan", SizeBytes: 20},
	}}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/system/volumes/orphaned/cleanup", `{"names":["db-gone-data","plain-orphan"]}`)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got cleanupOrphanedVolumesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Skipped) != 1 || got.Skipped[0] != "db-gone-data" {
		t.Errorf("Skipped = %v, want [db-gone-data]", got.Skipped)
	}
	if len(fake.removed) != 1 || fake.removed[0] != "plain-orphan" {
		t.Errorf("removed = %v, want only plain-orphan", fake.removed)
	}
}

func TestVolumeLockReclaimRefusesDatabaseVolume(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{volumes: []docker.NamedVolume{
		{Name: "db-gone-data", SizeBytes: 10},
		{Name: "plain-orphan", SizeBytes: 20},
	}}
	rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/storage/orphans/db-gone-data", `{"confirm":"db-gone-data"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.removed) != 0 {
		t.Fatalf("removed = %v, a database volume must not be removed", fake.removed)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/storage/orphans/plain-orphan", `{"confirm":"plain-orphan"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("ordinary orphan status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.removed) != 1 || fake.removed[0] != "plain-orphan" {
		t.Errorf("removed = %v, want plain-orphan", fake.removed)
	}
}

func TestVolumeLockAppDeleteDataStillPurgesAppVolumes(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{}
	rt, _ := newTestRouterWithOrphanedVolumeManager(t, fake)
	rt.purgeVolumes(context.Background(), "app/web", []string{"web-data"})
	if len(fake.removed) != 1 || fake.removed[0] != "web-data" {
		t.Errorf("removed = %v, want web-data", fake.removed)
	}
}

func TestVolumeLockJanitorRemoveRefusesDatabaseVolume(t *testing.T) {
	fake := &fakeOrphanedVolumeManager{}
	rt, _ := newTestRouterWithOrphanedVolumeManager(t, fake)
	if prot, _ := rt.volumeLock("db-x-data"); !prot {
		t.Error("db- volume must be locked")
	}
	if prot, _ := rt.volumeLock("web-data"); prot {
		t.Error("app volume must not be locked")
	}
}
