package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandlePauseResumeBackupSchedule(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	do := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, ""))
		return rec
	}

	if rec := do("/api/v1/databases/missing/backup-schedule/pause"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing database status = %d, want 404", rec.Code)
	}
	if rec := do("/api/v1/databases/main/backup-schedule/pause"); rec.Code != http.StatusConflict {
		t.Fatalf("no schedule status = %d, want 409", rec.Code)
	}

	target := seedBackupTargetForAPI(t, db)
	if err := db.SetDatabaseBackupSchedule(ctx, "main", target.ID, "0 3 * * *", 7, 0); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}

	rec := do("/api/v1/databases/main/backup-schedule/pause")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"0 3 * * *"`) {
		t.Fatalf("pause status = %d body = %s", rec.Code, rec.Body.String())
	}
	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil || !d.BackupPaused || d.BackupSchedule != "0 3 * * *" {
		t.Fatalf("after pause = %+v err = %v, want paused with schedule kept", d, err)
	}

	if rec := do("/api/v1/databases/main/backup-schedule/resume"); rec.Code != http.StatusOK {
		t.Fatalf("resume status = %d body = %s", rec.Code, rec.Body.String())
	}
	d, err = db.GetDesiredDatabase(ctx, "main")
	if err != nil || d.BackupPaused {
		t.Fatalf("after resume = %+v err = %v, want not paused", d, err)
	}
}
