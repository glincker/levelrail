package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func seedDatabaseBackupForRestoreTest(t *testing.T, db *store.DB, id, status string) {
	t.Helper()
	ctx := context.Background()
	target := seedBackupTargetForAPI(t, db)
	if err := db.StartBackupHistory(ctx, store.BackupHistory{ID: id, DatabaseName: "main", TargetID: target.ID, ObjectKey: "main/" + id, StartedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if status != store.BackupStatusRunning {
		if err := db.FinishBackupHistory(ctx, id, status, 10, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDatabaseRestoreTestStartsDrillOnLatestSucceeded(t *testing.T) {
	t.Parallel()
	p := &fakeBackupProtection{}
	rt, db := newProtectionRouter(t, p, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	seedDatabaseBackupForRestoreTest(t, db, "bkh_ok", store.BackupStatusSucceeded)

	rec := do(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/restore-test", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got restoreTestResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.BackupID != "bkh_ok" || got.Status != store.DrillStatusRunning || len(p.drillCalls) != 1 {
		t.Fatalf("got %+v calls %v", got, p.drillCalls)
	}
}

func TestDatabaseRestoreTestWithoutSucceededBackup(t *testing.T) {
	t.Parallel()
	p := &fakeBackupProtection{}
	rt, db := newProtectionRouter(t, p, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	seedDatabaseBackupForRestoreTest(t, db, "bkh_bad", store.BackupStatusFailed)
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/restore-test", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(p.drillCalls) != 0 {
		t.Fatal("no drill may start")
	}
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/restore-test", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get status = %d", rec.Code)
	}
}

func TestDatabaseRestorePreviewNamesSafetyBackup(t *testing.T) {
	t.Parallel()
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	seedDatabaseBackupForRestoreTest(t, db, "bkh_old", store.BackupStatusSucceeded)
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/restore-preview", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing backup_id status = %d", rec.Code)
	}
	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/restore-preview?backup_id=bkh_old", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got restorePreviewResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.BackupID != "bkh_old" || got.SizeBytes != 10 || got.AgeSeconds <= 0 || got.Busy || got.SafetyBackupID != "" || got.Overwrites == "" {
		t.Fatalf("got %+v", got)
	}
}
