package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeVolumeBackupRunner is a hand-written fake for ServiceVolumeBackupRunner,
// mirroring fakeBackupRunner's own shape exactly (backups_test.go).
type fakeVolumeBackupRunner struct {
	err   error
	calls chan volumeBackupRunCall
}

type volumeBackupRunCall struct {
	historyID, serviceName, volumeName, dockerVolumeName, targetID string
}

func newFakeVolumeBackupRunner() *fakeVolumeBackupRunner {
	return &fakeVolumeBackupRunner{calls: make(chan volumeBackupRunCall, 4)}
}

func (f *fakeVolumeBackupRunner) RunVolumeBackup(_ context.Context, historyID, serviceName, volumeName, dockerVolumeName, targetID string) error {
	f.calls <- volumeBackupRunCall{historyID, serviceName, volumeName, dockerVolumeName, targetID}
	return f.err
}

func (f *fakeVolumeBackupRunner) awaitCall(t *testing.T) volumeBackupRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunVolumeBackup was not called within the deadline")
		return volumeBackupRunCall{}
	}
}

func newTestRouterWithVolumeBackupRunner(t *testing.T, runner ServiceVolumeBackupRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithServiceVolumeBackupRunner(runner)), db
}

// seedServiceWithVolume seeds a service named "web" with one volume
// named "data": every test in this package that needs a volume-bearing
// service uses that exact pair, so neither is parameterized.
func seedServiceWithVolume(t *testing.T, db *store.DB) {
	t.Helper()
	const serviceName, logicalVolume = "web", "data"
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name:  serviceName,
		Image: "levelrail/thesvg:abc1234",
		Volumes: []store.ServiceVolume{
			{Name: "app-" + serviceName + "-" + logicalVolume, ContainerPath: "/data"},
		},
	}); err != nil {
		t.Fatalf("seed service %q: %v", serviceName, err)
	}
}

func TestVolumeBackupRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/apps/web/volumes/data/backups"},
		{http.MethodGet, "/api/v1/apps/web/volumes/data/backups"},
		{http.MethodPut, "/api/v1/apps/web/volumes/data/backup-schedule"},
		{http.MethodDelete, "/api/v1/apps/web/volumes/data/backup-schedule"},
		{http.MethodPost, "/api/v1/apps/web/volumes/data/restore"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleTriggerVolumeBackup_NoRunnerConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithServiceVolumeBackupRunner
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleTriggerVolumeBackup_ServiceNotFound(t *testing.T) {
	runner := newFakeVolumeBackupRunner()
	rt, db := newTestRouterWithVolumeBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/missing/volumes/data/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerVolumeBackup_VolumeNotFound(t *testing.T) {
	runner := newFakeVolumeBackupRunner()
	rt, db := newTestRouterWithVolumeBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/missing/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerVolumeBackup_Success(t *testing.T) {
	runner := newFakeVolumeBackupRunner()
	rt, db := newTestRouterWithVolumeBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got backupHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(got.ID, "bkh_") {
		t.Errorf("ID = %q, want a bkh_-prefixed id", got.ID)
	}
	if got.ServiceName != "web" || got.VolumeName != "data" || got.DatabaseName != "" {
		t.Errorf("response = %+v, want service_name=web volume_name=data, empty database_name", got)
	}

	call := runner.awaitCall(t)
	if call.serviceName != "web" || call.volumeName != "data" || call.dockerVolumeName != "app-web-data" || call.targetID != "bkt_test1" {
		t.Errorf("RunVolumeBackup call = %+v, want service=web volume=data dockerVolume=app-web-data target=bkt_test1", call)
	}
}

func TestHandleVolumeBackupSchedule_SetGetClear(t *testing.T) {
	runner := newFakeVolumeBackupRunner()
	rt, db := newTestRouterWithVolumeBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/volumes/data/backup-schedule", `{"target_id":"bkt_test1","schedule":"0 3 * * *","retain":5}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backup-schedule", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got volumeBackupScheduleResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.TargetID != "bkt_test1" || got.Schedule != "0 3 * * *" || got.Retain != 5 {
		t.Errorf("got = %+v, want the values just set", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/volumes/data/backup-schedule", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backup-schedule", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after clear status = %d, want %d", rec.Code, http.StatusOK)
	}
	got = volumeBackupScheduleResource{}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Schedule != "" || got.TargetID != "" {
		t.Errorf("got after clear = %+v, want an empty schedule", got)
	}
}

func TestHandleListVolumeBackupHistory_Success(t *testing.T) {
	runner := newFakeVolumeBackupRunner()
	rt, db := newTestRouterWithVolumeBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backups", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []backupHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "bkh_1" {
		t.Fatalf("got = %+v, want exactly one row for bkh_1", got)
	}
}
