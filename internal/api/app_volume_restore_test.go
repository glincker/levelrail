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

type fakeVolumeRestoreRunner struct {
	err   error
	calls chan volumeRestoreRunCall
}

type volumeRestoreRunCall struct {
	historyID, serviceName, volumeName, dockerVolumeName, backupHistoryID string
}

func newFakeVolumeRestoreRunner() *fakeVolumeRestoreRunner {
	return &fakeVolumeRestoreRunner{calls: make(chan volumeRestoreRunCall, 4)}
}

func (f *fakeVolumeRestoreRunner) RunVolumeRestore(_ context.Context, historyID, serviceName, volumeName, dockerVolumeName, backupHistoryID string) error {
	f.calls <- volumeRestoreRunCall{historyID, serviceName, volumeName, dockerVolumeName, backupHistoryID}
	return f.err
}

func (f *fakeVolumeRestoreRunner) awaitCall(t *testing.T) volumeRestoreRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunVolumeRestore was not called within the deadline")
		return volumeRestoreRunCall{}
	}
}

func newTestRouterWithVolumeRestoreRunner(t *testing.T, runner ServiceVolumeRestoreRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithServiceVolumeRestoreRunner(runner)), db
}

func TestHandleTriggerVolumeRestore_NoRunnerConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db, "web", "data")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleTriggerVolumeRestore_BackupNotFound(t *testing.T) {
	runner := newFakeVolumeRestoreRunner()
	rt, db := newTestRouterWithVolumeRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db, "web", "data")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore", `{"backup_id":"bkh_missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerVolumeRestore_BackupBelongsToDifferentVolume(t *testing.T) {
	runner := newFakeVolumeRestoreRunner()
	rt, db := newTestRouterWithVolumeRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name:  "web",
		Image: "levelrail/thesvg:abc1234",
		Volumes: []store.ServiceVolume{
			{Name: "app-web-data", ContainerPath: "/data"},
			{Name: "app-web-cache", ContainerPath: "/cache"},
		},
	}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "cache",
		TargetID: target.ID, ObjectKey: "volumes/web/cache/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), "bkh_1", store.BackupStatusSucceeded, 10, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("finish backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (backup belongs to a different volume), body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleTriggerVolumeRestore_RefusesUnsuccessfulBackup(t *testing.T) {
	runner := newFakeVolumeRestoreRunner()
	rt, db := newTestRouterWithVolumeRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db, "web", "data")
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	// Left running, never finished: not eligible to restore from.

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleTriggerVolumeRestore_Success(t *testing.T) {
	runner := newFakeVolumeRestoreRunner()
	rt, db := newTestRouterWithVolumeRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db, "web", "data")
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), "bkh_1", store.BackupStatusSucceeded, 10, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("finish backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got restoreHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(got.ID, "rsh_") {
		t.Errorf("ID = %q, want an rsh_-prefixed id", got.ID)
	}
	if got.ServiceName != "web" || got.VolumeName != "data" {
		t.Errorf("response = %+v, want service_name=web volume_name=data", got)
	}

	call := runner.awaitCall(t)
	if call.serviceName != "web" || call.volumeName != "data" || call.dockerVolumeName != "app-web-data" || call.backupHistoryID != "bkh_1" {
		t.Errorf("RunVolumeRestore call = %+v, want service=web volume=data dockerVolume=app-web-data backupID=bkh_1", call)
	}
}

func TestHandleListVolumeRestoreHistory_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db, "web", "data")
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	if err := db.StartRestoreHistory(context.Background(), store.RestoreHistory{
		ID: "rsh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		BackupHistoryID: "bkh_1", StartedAt: "2026-08-14T00:02:00Z",
	}); err != nil {
		t.Fatalf("seed restore history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/restores", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []restoreHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "rsh_1" {
		t.Fatalf("got = %+v, want exactly one row for rsh_1", got)
	}
}
