package api

import (
	"context"
	"errors"
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

func TestHandleDownloadVolumeBackup_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/volumes/data/backups/bkh_1/download", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDownloadVolumeBackup_NoDownloaderConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBackupDownloader
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backups/bkh_1/download", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDownloadVolumeBackup_ServiceNotFound(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "tar-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/volumes/data/backups/bkh_1/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0", downloader.callCount)
	}
}

func TestHandleDownloadVolumeBackup_VolumeNotFound(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "tar-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/missing/backups/bkh_1/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0", downloader.callCount)
	}
}

func TestHandleDownloadVolumeBackup_BackupNotFound(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "tar-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backups/bkh_missing/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0", downloader.callCount)
	}
}

func TestHandleDownloadVolumeBackup_NotSucceeded(t *testing.T) {
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
	// Do not FinishBackupHistory, so it remains running (not succeeded)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/volumes/data/backups/bkh_1/download", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0", downloader.callCount)
	}
}

func TestHandleDownloadVolumeBackup_DownloaderFails(t *testing.T) {
	downloader := &fakeBackupDownloader{err: errors.New("s3 error")}
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
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestVolumeDownloadFilename(t *testing.T) {
	tests := []struct {
		name      string
		h         store.BackupHistory
		want      string
	}{
		{
			name: "extracts base from path",
			h: store.BackupHistory{
				ServiceName: "web", VolumeName: "data",
				ObjectKey: "volumes/web/data/my-backup-file.tar.gz",
				StartedAt: "2026-08-14T03:00:00Z",
			},
			want: "my-backup-file.tar.gz",
		},
		{
			name: "fallback when ObjectKey is empty",
			h: store.BackupHistory{
				ServiceName: "web", VolumeName: "data",
				ObjectKey: "",
				StartedAt: "2026-08-14T03:00:00Z",
			},
			want: "web-data-2026-08-14T030000Z.tar",
		},
		{
			name: "fallback when ObjectKey is slash",
			h: store.BackupHistory{
				ServiceName: "web", VolumeName: "data",
				ObjectKey: "/",
				StartedAt: "2026-08-14T03:00:00Z",
			},
			want: "web-data-2026-08-14T030000Z.tar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := volumeDownloadFilename(tc.h); got != tc.want {
				t.Errorf("volumeDownloadFilename() = %q, want %q", got, tc.want)
			}
		})
	}
}
