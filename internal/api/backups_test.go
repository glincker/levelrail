package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBackupRunner is a hand-written fake for BackupRunner, the same
// pattern every other test in this package uses. calls is buffered so
// handleTriggerBackup's detached goroutine (see its own doc comment for
// why it must not run on r.Context()) can send without blocking even if
// a test never reads it.
type fakeBackupRunner struct {
	err   error
	calls chan backupRunCall
}

type backupRunCall struct {
	historyID, databaseName, engine, containerName, targetID string
}

func newFakeBackupRunner() *fakeBackupRunner {
	return &fakeBackupRunner{calls: make(chan backupRunCall, 4)}
}

func (f *fakeBackupRunner) RunBackup(_ context.Context, historyID, databaseName, engine, containerName, targetID string) error {
	f.calls <- backupRunCall{historyID, databaseName, engine, containerName, targetID}
	return f.err
}

// awaitCall waits up to a short deadline for RunBackup to have been
// called, rather than a fixed sleep: handleTriggerBackup starts it in a
// goroutine specifically so the HTTP response doesn't wait for it, so a
// test asserting it happened has to synchronize on something real.
func (f *fakeBackupRunner) awaitCall(t *testing.T) backupRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunBackup was not called within the deadline")
		return backupRunCall{}
	}
}

func newTestRouterWithBackupRunner(t *testing.T, runner BackupRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithBackupRunner(runner)), db
}

func seedBackupTargetForAPI(t *testing.T, db *store.DB) store.BackupTarget {
	t.Helper()
	target := store.BackupTarget{
		ID: "bkt_test1", Name: "primary", Provider: store.BackupProviderAWS,
		Bucket: "backups", CreatedAt: "2026-08-14T00:00:00Z",
	}
	if err := db.SaveBackupTarget(context.Background(), target); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}
	return target
}

func TestBackupRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/databases/main/backups"},
		{http.MethodGet, "/api/v1/databases/main/backups"},
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

func TestHandleTriggerBackup_NoRunnerConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBackupRunner
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleTriggerBackup_DatabaseNotFound(t *testing.T) {
	runner := newFakeBackupRunner()
	rt, db := newTestRouterWithBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/missing/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerBackup_MissingTargetID(t *testing.T) {
	runner := newFakeBackupRunner()
	rt, db := newTestRouterWithBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleTriggerBackup_TargetNotFound(t *testing.T) {
	runner := newFakeBackupRunner()
	rt, db := newTestRouterWithBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups", `{"target_id":"bkt_missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerBackup_Success(t *testing.T) {
	runner := newFakeBackupRunner()
	rt, db := newTestRouterWithBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups", `{"target_id":"bkt_test1"}`))
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
	if got.Status != store.BackupStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, store.BackupStatusRunning)
	}
	if got.DatabaseName != "main" || got.TargetID != "bkt_test1" {
		t.Errorf("response = %+v, want database_name=main target_id=bkt_test1", got)
	}

	call := runner.awaitCall(t)
	if call.historyID != got.ID {
		t.Errorf("RunBackup historyID = %q, want it to match the response id %q", call.historyID, got.ID)
	}
	if call.databaseName != "main" {
		t.Errorf("RunBackup databaseName = %q, want %q", call.databaseName, "main")
	}
	if call.engine != store.EnginePostgres {
		t.Errorf("RunBackup engine = %q, want %q", call.engine, store.EnginePostgres)
	}
	if call.containerName != "db-main" {
		t.Errorf("RunBackup containerName = %q, want %q", call.containerName, "db-main")
	}
	if call.targetID != "bkt_test1" {
		t.Errorf("RunBackup targetID = %q, want %q", call.targetID, "bkt_test1")
	}
}

func TestBackupScheduleRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPut, "/api/v1/databases/main/backup-schedule"},
		{http.MethodDelete, "/api/v1/databases/main/backup-schedule"},
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

func TestHandleSetBackupSchedule_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/missing/backup-schedule", `{"target_id":"bkt_test1","schedule":"0 3 * * *"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetBackupSchedule_MissingTargetID(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/backup-schedule", `{"schedule":"0 3 * * *"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetBackupSchedule_MissingSchedule(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/backup-schedule", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetBackupSchedule_InvalidCron(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/backup-schedule", `{"target_id":"bkt_test1","schedule":"not a cron string"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetBackupSchedule_NegativeRetain(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/backup-schedule", `{"target_id":"bkt_test1","schedule":"0 3 * * *","retain":-1}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetBackupSchedule_TargetNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/backup-schedule", `{"target_id":"bkt_missing","schedule":"0 3 * * *"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSetBackupSchedule_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/backup-schedule", `{"target_id":"bkt_test1","schedule":"0 3 * * *","retain":7}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got backupScheduleResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.DatabaseName != "main" || got.TargetID != "bkt_test1" || got.Schedule != "0 3 * * *" || got.Retain != 7 {
		t.Errorf("response = %+v, want database_name=main target_id=bkt_test1 schedule=\"0 3 * * *\" retain=7", got)
	}

	saved, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if saved.BackupTargetID != "bkt_test1" || saved.BackupSchedule != "0 3 * * *" || saved.BackupRetain != 7 {
		t.Errorf("persisted database = %+v, want the schedule to be saved", saved)
	}
}

func TestHandleClearBackupSchedule_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/missing/backup-schedule", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleClearBackupSchedule_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)
	if err := db.SetDatabaseBackupSchedule(context.Background(), "main", target.ID, "0 3 * * *", 7); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/backup-schedule", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	saved, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if saved.BackupTargetID != "" || saved.BackupSchedule != "" || saved.BackupRetain != 0 {
		t.Errorf("persisted database after clear = %+v, want all three fields zero-valued", saved)
	}
}

func TestHandleListBackupHistory_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/backups", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListBackupHistory_NoRunnerConfigured_StillWorks(t *testing.T) {
	// Listing history needs no runner: see WithBackupRunner's own doc
	// comment. This test locks that in.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)
	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", DatabaseName: "main", TargetID: target.ID,
		ObjectKey: "main/main-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []backupHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "bkh_1" {
		t.Fatalf("history = %+v, want exactly the seeded row", got)
	}
}

// TestHandleListBackupHistory_Pagination mirrors
// TestAuditLogRoute_Pagination (audit_test.go): ?limit caps a page,
// ?before pages backward through the rest without repeating rows.
func TestHandleListBackupHistory_Pagination(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"bkh_seed_1", "bkh_seed_2", "bkh_seed_3"} {
		if err := db.StartBackupHistory(ctx, store.BackupHistory{
			ID: id, DatabaseName: "main", TargetID: target.ID,
			ObjectKey: id, StartedAt: base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("seed backup history %s: %v", id, err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups?limit=2", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var page1 []backupHistoryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &page1); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page1) != 2 || page1[0].ID != "bkh_seed_3" || page1[1].ID != "bkh_seed_2" {
		t.Fatalf("page1 = %+v, want [bkh_seed_3, bkh_seed_2]", page1)
	}

	before := page1[len(page1)-1].StartedAt
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups?before="+url.QueryEscape(before), ""))
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec2.Code, http.StatusOK, rec2.Body.String())
	}
	var page2 []backupHistoryResource
	if err := json.Unmarshal(rec2.Body.Bytes(), &page2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page2) != 1 || page2[0].ID != "bkh_seed_1" {
		t.Fatalf("page2 = %+v, want exactly [bkh_seed_1]", page2)
	}
}

func TestHandleListBackupHistory_InvalidLimit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups?limit=not-a-number", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListBackupHistory_InvalidBefore(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups?before=not-a-timestamp", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
