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

// fakeVolumeCloneRestoreRunner is a hand-written fake for
// VolumeCloneRestoreRunner, the same pattern fakeCloneRestoreRunner
// (database_clone_restore_test.go) already establishes for the database
// resource kind.
type fakeVolumeCloneRestoreRunner struct {
	err   error
	calls chan volumeCloneRestoreRunCall
}

type volumeCloneRestoreRunCall struct {
	historyID, sourceServiceName, sourceVolumeName, newVolumeName, backupHistoryID string
}

func newFakeVolumeCloneRestoreRunner() *fakeVolumeCloneRestoreRunner {
	return &fakeVolumeCloneRestoreRunner{calls: make(chan volumeCloneRestoreRunCall, 4)}
}

func (f *fakeVolumeCloneRestoreRunner) RunVolumeCloneRestore(_ context.Context, historyID, sourceServiceName, sourceVolumeName, newVolumeName, backupHistoryID string) error {
	f.calls <- volumeCloneRestoreRunCall{historyID, sourceServiceName, sourceVolumeName, newVolumeName, backupHistoryID}
	return f.err
}

func (f *fakeVolumeCloneRestoreRunner) awaitCall(t *testing.T) volumeCloneRestoreRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunVolumeCloneRestore was not called within the deadline")
		return volumeCloneRestoreRunCall{}
	}
}

func newTestRouterWithVolumeCloneRestoreRunner(t *testing.T, runner VolumeCloneRestoreRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithVolumeCloneRestoreRunner(runner)), db
}

// seedSucceededVolumeBackupForCloneRestore seeds a succeeded backup of
// "web"'s volumeName (every test in this file uses seedServiceWithVolume's
// own fixed service name), the volume counterpart of
// seedSucceededBackupForCloneRestore (database_clone_restore_test.go).
func seedSucceededVolumeBackupForCloneRestore(t *testing.T, db *store.DB, volumeName string) store.BackupHistory {
	t.Helper()
	const serviceName = "web"
	target := seedBackupTargetForAPI(t, db)
	h := store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: serviceName, VolumeName: volumeName,
		TargetID: target.ID, ObjectKey: "volumes/" + serviceName + "/" + volumeName + "/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}
	if err := db.StartBackupHistory(context.Background(), h); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), h.ID, store.BackupStatusSucceeded, 10, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}
	return h
}

func TestVolumeCloneRestoreRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new"},
		{http.MethodGet, "/api/v1/apps/web/volumes/data/clone-restores"},
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

func TestHandleVolumeCloneRestore_NoRunnerConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithVolumeCloneRestoreRunner
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleVolumeCloneRestore_ServiceNotFound(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/missing/volumes/data/restore-as-new", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleVolumeCloneRestore_MissingBackupID(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleVolumeCloneRestore_BackupNotFound(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{"backup_id":"bkh_missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleVolumeCloneRestore_BackupFromDifferentVolume(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
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
	seedSucceededVolumeBackupForCloneRestore(t, db, "cache")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (backup belongs to a different volume), body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleVolumeCloneRestore_BackupNotSucceeded(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

// TestHandleVolumeCloneRestore_Success_DefaultsNewVolumeName is the core
// happy path with no new_volume_name supplied: the server mints one
// itself (defaultVolumeCloneName), and the source service/volume are
// never touched (there is nothing for this handler to write to them:
// only rt.backupHistory.GetBackupHistory is ever read).
func TestHandleVolumeCloneRestore_Success_DefaultsNewVolumeName(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedSucceededVolumeBackupForCloneRestore(t, db, "data")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got volumeCloneRestoreResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(got.ID, "vcr_") {
		t.Errorf("ID = %q, want a vcr_-prefixed id", got.ID)
	}
	if got.Status != store.BackupStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, store.BackupStatusRunning)
	}
	if got.SourceServiceName != "web" || got.SourceVolumeName != "data" || got.BackupHistoryID != "bkh_1" {
		t.Errorf("response = %+v, want source_service_name=web source_volume_name=data backup_history_id=bkh_1", got)
	}
	if !strings.HasPrefix(got.NewVolumeName, "clone-web-data-") {
		t.Errorf("NewVolumeName = %q, want a generated clone-web-data-* name", got.NewVolumeName)
	}

	call := runner.awaitCall(t)
	if call.historyID != got.ID {
		t.Errorf("RunVolumeCloneRestore historyID = %q, want it to match the response id %q", call.historyID, got.ID)
	}
	if call.sourceServiceName != "web" || call.sourceVolumeName != "data" {
		t.Errorf("RunVolumeCloneRestore source = %q/%q, want web/data", call.sourceServiceName, call.sourceVolumeName)
	}
	if call.newVolumeName != got.NewVolumeName {
		t.Errorf("RunVolumeCloneRestore newVolumeName = %q, want %q", call.newVolumeName, got.NewVolumeName)
	}
	if call.backupHistoryID != "bkh_1" {
		t.Errorf("RunVolumeCloneRestore backupHistoryID = %q, want bkh_1", call.backupHistoryID)
	}
}

// TestHandleVolumeCloneRestore_Success_ExplicitNewVolumeName proves an
// operator-supplied new_volume_name passes straight through, unmodified.
func TestHandleVolumeCloneRestore_Success_ExplicitNewVolumeName(t *testing.T) {
	runner := newFakeVolumeCloneRestoreRunner()
	rt, db := newTestRouterWithVolumeCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedSucceededVolumeBackupForCloneRestore(t, db, "data")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/restore-as-new", `{"backup_id":"bkh_1","new_volume_name":"web-data-inspect"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got volumeCloneRestoreResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.NewVolumeName != "web-data-inspect" {
		t.Errorf("NewVolumeName = %q, want the operator-supplied value", got.NewVolumeName)
	}

	call := runner.awaitCall(t)
	if call.newVolumeName != "web-data-inspect" {
		t.Errorf("RunVolumeCloneRestore newVolumeName = %q, want web-data-inspect", call.newVolumeName)
	}
}

func TestHandleListVolumeCloneRestores_ServiceNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/volumes/data/clone-restores", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListVolumeCloneRestores_NoRunnerConfigured_StillWorks(t *testing.T) {
	rt, db := newTestRouter(t) // no WithVolumeCloneRestoreRunner
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	backup := seedSucceededVolumeBackupForCloneRestore(t, db, "data")
	if err := db.StartVolumeCloneRestore(context.Background(), store.VolumeCloneRestore{
		ID: "vcr_1", SourceServiceName: "web", SourceVolumeName: "data", NewVolumeName: "clone-web-data-abc123",
		BackupHistoryID: backup.ID, StartedAt: "2026-08-14T02:00:00Z",
	}); err != nil {
		t.Fatalf("seed volume clone restore: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/clone-restores", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []volumeCloneRestoreResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "vcr_1" {
		t.Fatalf("history = %+v, want exactly the seeded row", got)
	}
}
