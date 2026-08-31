package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleDownloadVolumeBackup_Success(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "tar-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backups/bkh_1/download", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != "tar-bytes" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "tar-bytes")
	}
	if downloader.gotHistory != "bkh_1" {
		t.Errorf("Downloader received history id %q, want %q", downloader.gotHistory, "bkh_1")
	}
}

func TestHandleDownloadVolumeBackup_WrongVolume(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "tar-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backups/bkh_1/download", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (backup belongs to a different volume)", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleVerifyVolumeBackup_Success(t *testing.T) {
	verifier := &fakeBackupVerifier{}
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/volumes/data/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var got backupVerificationResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.BackupHistoryID != "bkh_1" {
		t.Errorf("BackupHistoryID = %q, want %q", got.BackupHistoryID, "bkh_1")
	}
}
