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

// fakeCloneRestoreRunner is a hand-written fake for CloneRestoreRunner,
// the same pattern fakeBackupRunner (backups_test.go) already
// establishes for its own runner interface.
type fakeCloneRestoreRunner struct {
	err   error
	calls chan cloneRestoreRunCall
}

type cloneRestoreRunCall struct {
	historyID, sourceDatabaseName, newDatabaseName, backupHistoryID, engine, containerName, controllerName string
}

func newFakeCloneRestoreRunner() *fakeCloneRestoreRunner {
	return &fakeCloneRestoreRunner{calls: make(chan cloneRestoreRunCall, 4)}
}

func (f *fakeCloneRestoreRunner) RunCloneRestore(_ context.Context, historyID, sourceDatabaseName, newDatabaseName, backupHistoryID, engine, containerName, controllerName string) error {
	f.calls <- cloneRestoreRunCall{historyID, sourceDatabaseName, newDatabaseName, backupHistoryID, engine, containerName, controllerName}
	return f.err
}

func (f *fakeCloneRestoreRunner) awaitCall(t *testing.T) cloneRestoreRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunCloneRestore was not called within the deadline")
		return cloneRestoreRunCall{}
	}
}

func newTestRouterWithCloneRestoreRunner(t *testing.T, runner CloneRestoreRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithCloneRestoreRunner(runner)), db
}

func seedSucceededBackupForCloneRestore(t *testing.T, db *store.DB, databaseName string) store.BackupHistory {
	t.Helper()
	target := seedBackupTargetForAPI(t, db)
	h := store.BackupHistory{
		ID: "bkh_1", DatabaseName: databaseName, TargetID: target.ID,
		ObjectKey: databaseName + "/1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}
	if err := db.StartBackupHistory(context.Background(), h); err != nil {
		t.Fatalf("StartBackupHistory() error = %v", err)
	}
	if err := db.FinishBackupHistory(context.Background(), h.ID, store.BackupStatusSucceeded, 1, "", "", "2026-08-14T00:01:00Z"); err != nil {
		t.Fatalf("FinishBackupHistory() error = %v", err)
	}
	return h
}

func TestCloneRestoreRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/databases/main/restore-as-new"},
		{http.MethodGet, "/api/v1/databases/main/clone-restores"},
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

func TestHandleCloneRestore_NoRunnerConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithCloneRestoreRunner
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleCloneRestore_SourceDatabaseNotFound(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/missing/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCloneRestore_MissingBackupID(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"new_name":"main-staging"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneRestore_MissingNewName(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneRestore_NewNameSameAsSource(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneRestore_BackupNotFound(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_missing","new_name":"main-staging"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCloneRestore_BackupFromDifferentDatabase(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed main: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "other", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "other")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneRestore_BackupNotSucceeded(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)
	if err := db.StartBackupHistory(context.Background(), store.BackupHistory{
		ID: "bkh_1", DatabaseName: "main", TargetID: target.ID,
		ObjectKey: "main/1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleCloneRestore_NewNameAlreadyExists(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed main: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main-staging", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed main-staging: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

// TestHandleCloneRestore_Success is the core happy path: the new database
// is created (through the same createDesiredDatabase path
// handleCreateDatabase itself uses), the clone-restore runner is invoked
// with the source's own engine, and the source database itself is never
// modified.
func TestHandleCloneRestore_Success(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got cloneRestoreResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(got.ID, "clr_") {
		t.Errorf("ID = %q, want a clr_-prefixed id", got.ID)
	}
	if got.Status != store.BackupStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, store.BackupStatusRunning)
	}
	if got.SourceDatabaseName != "main" || got.NewDatabaseName != "main-staging" || got.BackupHistoryID != "bkh_1" {
		t.Errorf("response = %+v, want source_database_name=main new_database_name=main-staging backup_history_id=bkh_1", got)
	}

	// The new database now exists, created through the ordinary path,
	// with the source's own engine and version copied over.
	newDB, err := db.GetDesiredDatabase(context.Background(), "main-staging")
	if err != nil {
		t.Fatalf("GetDesiredDatabase(main-staging) error = %v", err)
	}
	if newDB.Engine != store.EnginePostgres || newDB.Version != "16" {
		t.Errorf("new database = %+v, want engine=postgres version=16 copied from the source", newDB)
	}

	// The source database itself is completely untouched.
	sourceDB, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase(main) error = %v", err)
	}
	if sourceDB.Engine != store.EnginePostgres || sourceDB.Version != "16" {
		t.Errorf("source database = %+v, want it unchanged", sourceDB)
	}

	call := runner.awaitCall(t)
	if call.historyID != got.ID {
		t.Errorf("RunCloneRestore historyID = %q, want it to match the response id %q", call.historyID, got.ID)
	}
	if call.sourceDatabaseName != "main" || call.newDatabaseName != "main-staging" {
		t.Errorf("RunCloneRestore source/new = %q/%q, want main/main-staging", call.sourceDatabaseName, call.newDatabaseName)
	}
	if call.backupHistoryID != "bkh_1" {
		t.Errorf("RunCloneRestore backupHistoryID = %q, want bkh_1", call.backupHistoryID)
	}
	if call.engine != store.EnginePostgres {
		t.Errorf("RunCloneRestore engine = %q, want %q", call.engine, store.EnginePostgres)
	}
	if call.containerName != "db-main-staging" {
		t.Errorf("RunCloneRestore containerName = %q, want %q", call.containerName, "db-main-staging")
	}
	if call.controllerName != "database/main-staging" {
		t.Errorf("RunCloneRestore controllerName = %q, want %q", call.controllerName, "database/main-staging")
	}
}

// TestHandleCloneRestore_WithResources proves req.Resources is applied to
// the new database as a trailing step, the same way handleCreateDatabase
// itself never accepts resources directly (databaseResource.Resources'
// own field doc comment) but handleSetDatabaseResources does.
func TestHandleCloneRestore_WithResources(t *testing.T) {
	runner := newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"main-staging","resources":{"memory_bytes":536870912}}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	newDB, err := db.GetDesiredDatabase(context.Background(), "main-staging")
	if err != nil {
		t.Fatalf("GetDesiredDatabase(main-staging) error = %v", err)
	}
	if newDB.Resources == nil || newDB.Resources.MemoryBytes != 536870912 {
		t.Errorf("new database resources = %+v, want MemoryBytes=536870912", newDB.Resources)
	}

	runner.awaitCall(t)
}

func TestHandleListCloneRestores_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/clone-restores", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListCloneRestores_NoRunnerConfigured_StillWorks(t *testing.T) {
	rt, db := newTestRouter(t) // no WithCloneRestoreRunner
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	backup := seedSucceededBackupForCloneRestore(t, db, "main")
	if err := db.StartCloneRestore(context.Background(), store.CloneRestore{
		ID: "clr_1", SourceDatabaseName: "main", NewDatabaseName: "main-staging",
		BackupHistoryID: backup.ID, StartedAt: "2026-08-14T02:00:00Z",
	}); err != nil {
		t.Fatalf("seed clone restore: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/clone-restores", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []cloneRestoreResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "clr_1" {
		t.Fatalf("history = %+v, want exactly the seeded row", got)
	}
}
