package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBackupDownloader is a hand-written fake for BackupDownloader, the
// same pattern fakeBackupRunner (backups_test.go) already establishes.
type fakeBackupDownloader struct {
	content    string
	err        error
	gotHistory string
	callCount  int
}

func (f *fakeBackupDownloader) Download(_ context.Context, historyID string) (io.ReadCloser, error) {
	f.callCount++
	f.gotHistory = historyID
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

func newTestRouterWithBackupDownloader(t *testing.T, downloader BackupDownloader) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithBackupDownloader(downloader)), db
}

// seedSucceededBackupForAPI writes a store.BackupHistory row directly
// (bypassing Runner entirely, the same "seed the row a handler needs to
// find" shortcut seedBackupTargetForAPI already takes for backup targets)
// so download tests can exercise handleDownloadBackup without a real
// Runner ever having run.
func seedSucceededBackupForAPI(t *testing.T, db *store.DB, id, databaseName string) {
	t.Helper()
	h := store.BackupHistory{
		ID:           id,
		DatabaseName: databaseName,
		TargetID:     "bkt_test1",
		ObjectKey:    databaseName + "/" + databaseName + "-20260814T030000Z.dump",
		StartedAt:    "2026-08-14T03:00:00Z",
	}
	if err := db.StartBackupHistory(context.Background(), h); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), id, store.BackupStatusSucceeded, 1024, "", "2026-08-14T03:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}
}

func TestHandleDownloadBackup_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/main/backups/bkh_1/download", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDownloadBackup_NoDownloaderConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBackupDownloader
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_1/download", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDownloadBackup_DatabaseNotFound(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "dump-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/backups/bkh_1/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0 for a database that doesn't exist", downloader.callCount)
	}
}

func TestHandleDownloadBackup_BackupNotFound(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "dump-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_missing/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0 for a backup that doesn't exist", downloader.callCount)
	}
}

func TestHandleDownloadBackup_BelongsToDifferentDatabase(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "dump-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	for _, name := range []string{"main", "other"} {
		if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "bkh_1", "other")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_1/download", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0 for a backup belonging to a different database", downloader.callCount)
	}
}

func TestHandleDownloadBackup_NotSucceeded(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "dump-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
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
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_1/download", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if downloader.callCount != 0 {
		t.Errorf("Downloader was called %d times, want 0 for a backup that hasn't succeeded", downloader.callCount)
	}
}

func TestHandleDownloadBackup_Success(t *testing.T) {
	downloader := &fakeBackupDownloader{content: "dump-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "bkh_1", "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_1/download", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != "dump-bytes" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "dump-bytes")
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", got)
	}
	wantDisposition := `attachment; filename="main-20260814T030000Z.dump"`
	if got := rec.Header().Get("Content-Disposition"); got != wantDisposition {
		t.Errorf("Content-Disposition = %q, want %q", got, wantDisposition)
	}
	if downloader.gotHistory != "bkh_1" {
		t.Errorf("Download called with historyID = %q, want %q", downloader.gotHistory, "bkh_1")
	}
}

func TestHandleDownloadBackup_DownloaderFails(t *testing.T) {
	downloader := &fakeBackupDownloader{err: errors.New("s3: object not found")}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "bkh_1", "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_1/download", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
