package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBackupDeleter is a hand-written fake for BackupDeleter, the same
// pattern fakeBackupDownloader (backup_download_test.go) already
// establishes.
type fakeBackupDeleter struct {
	err        error
	gotHistory string
	callCount  int
}

func (f *fakeBackupDeleter) DeleteBackup(_ context.Context, historyID string) error {
	f.callCount++
	f.gotHistory = historyID
	return f.err
}

func newTestRouterWithBackupDeleter(t *testing.T, deleter BackupDeleter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithBackupDeleter(deleter)), db
}

func TestDeleteBackupRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodDelete, "/api/v1/databases/main/backups/bkh_1"},
		{http.MethodDelete, "/api/v1/apps/web/volumes/data/backups/bkh_1"},
	})
}

func TestHandleDeleteBackup_NoDeleterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBackupDeleter
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_1", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDeleteBackup_DatabaseNotFound(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/missing/backups/bkh_1", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if deleter.callCount != 0 {
		t.Errorf("DeleteBackup was called %d times, want 0 for a database that doesn't exist", deleter.callCount)
	}
}

func TestHandleDeleteBackup_BackupNotFound(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_missing", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if deleter.callCount != 0 {
		t.Errorf("DeleteBackup was called %d times, want 0 for a backup that doesn't exist", deleter.callCount)
	}
}

func TestHandleDeleteBackup_BelongsToDifferentDatabase(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	for _, name := range []string{"main", "other"} {
		if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "other")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_1", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if deleter.callCount != 0 {
		t.Errorf("DeleteBackup was called %d times, want 0 for a backup belonging to a different database", deleter.callCount)
	}
}

func TestHandleDeleteBackup_StillRunning_Refused(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)
	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", DatabaseName: "main", TargetID: "bkt_test1",
		ObjectKey: "main/main-20260814T030000Z.dump", StartedAt: "2026-08-14T03:00:00Z",
	}); err != nil {
		t.Fatalf("seed running backup: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_1", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if deleter.callCount != 0 {
		t.Errorf("DeleteBackup was called %d times, want 0 for a backup that is still running", deleter.callCount)
	}
}

func TestHandleDeleteBackup_Success(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if deleter.gotHistory != "bkh_1" {
		t.Errorf("DeleteBackup called with historyID = %q, want %q", deleter.gotHistory, "bkh_1")
	}
}

// TestHandleDeleteBackup_FailedAttempt_CanStillBeDeleted covers the
// case retention pruning never touches (PruneBackupHistory's own doc
// comment: only succeeded rows are ever eligible), which this on-demand
// route deliberately allows: an operator cleaning up a failed attempt's
// clutter, not just succeeded archives.
func TestHandleDeleteBackup_FailedAttempt_CanStillBeDeleted(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)
	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", DatabaseName: "main", TargetID: target.ID,
		ObjectKey: "main/main-20260814T030000Z.dump", StartedAt: "2026-08-14T03:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), "bkh_1", store.BackupStatusFailed, 0, "", "upload: access denied", "2026-08-14T03:01:00Z"); err != nil {
		t.Fatalf("finish backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleDeleteBackup_DeleterFails(t *testing.T) {
	deleter := &fakeBackupDeleter{err: errors.New("store: disk full")}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backups/bkh_1", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleDeleteVolumeBackup_Success(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	target := seedBackupTargetForAPI(t, db)

	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data",
		TargetID: target.ID, ObjectKey: "volumes/web/data/1.tar", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), "bkh_1", store.BackupStatusSucceeded, 9, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("finish backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/volumes/data/backups/bkh_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if deleter.gotHistory != "bkh_1" {
		t.Errorf("DeleteBackup called with historyID = %q, want %q", deleter.gotHistory, "bkh_1")
	}
}

func TestHandleDeleteVolumeBackup_WrongVolume(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "levelrail/thesvg:abc1234",
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
	if err := db.FinishBackupHistory(context.Background(), "bkh_1", store.BackupStatusSucceeded, 9, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("finish backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/volumes/data/backups/bkh_1", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (backup belongs to a different volume)", rec.Code, http.StatusBadRequest)
	}
	if deleter.callCount != 0 {
		t.Errorf("DeleteBackup was called %d times, want 0 for a backup belonging to a different volume", deleter.callCount)
	}
}

func TestHandleDeleteVolumeBackup_StillRunning_Refused(t *testing.T) {
	deleter := &fakeBackupDeleter{}
	rt, db := newTestRouterWithBackupDeleter(t, deleter)
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/volumes/data/backups/bkh_1", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if deleter.callCount != 0 {
		t.Errorf("DeleteBackup was called %d times, want 0 for a backup that is still running", deleter.callCount)
	}
}
