package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newTestRouterWithCloneNow(t *testing.T, backupRunner BackupRunner, cloneRunner CloneRestoreRunner) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouterWithBackupRunner(t, backupRunner)
	rt.cloneRestoreRunner = cloneRunner
	return rt, db
}

// seedDatabaseWithBackupTarget seeds a "mydb" database with a real
// backup target configured, the shared precondition every clone-now
// test (and the preview-database-cloning tests in
// preview_environments_database_test.go) needs.
func seedDatabaseWithBackupTarget(t *testing.T, db *store.DB) store.BackupTarget {
	t.Helper()
	const name = "mydb"
	target := seedBackupTargetForAPI(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase(%q) error = %v", name, err)
	}
	if err := db.SetDatabaseBackupSchedule(context.Background(), name, target.ID, "0 3 * * *", 0, 0); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule(%q) error = %v", name, err)
	}
	return target
}

func TestHandleCloneDatabaseNow_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no backup/clone-restore runner
	cookie := loginTestSession(t, rt, db)
	seedDatabaseWithBackupTarget(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{"new_name":"mydb-clone"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleCloneDatabaseNow_MissingNewName(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)
	seedDatabaseWithBackupTarget(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneDatabaseNow_NewNameSameAsSource(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)
	seedDatabaseWithBackupTarget(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{"new_name":"mydb"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneDatabaseNow_NoTargetConfigured(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)
	// No backup schedule/target set for this one, unlike
	// seedDatabaseWithBackupTarget's helper.
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "mydb", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{"new_name":"mydb-clone"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCloneDatabaseNow_SourceNotFound(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/missing/clone", `{"new_name":"clone1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleCloneDatabaseNow_NewNameAlreadyExists(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)
	seedDatabaseWithBackupTarget(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "taken", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{"new_name":"taken"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleCloneDatabaseNow_Success_DefaultsTargetFromSource(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)
	target := seedDatabaseWithBackupTarget(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{"new_name":"mydb-clone"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got cloneNowResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SourceDatabaseName != "mydb" || got.NewDatabaseName != "mydb-clone" {
		t.Errorf("got = %+v, want source=mydb new=mydb-clone", got)
	}
	if got.TargetID != target.ID {
		t.Errorf("TargetID = %q, want the source database's own backup target %q", got.TargetID, target.ID)
	}

	backupCall := backupRunner.awaitCall(t)
	if backupCall.databaseName != "mydb" || backupCall.targetID != target.ID {
		t.Errorf("backup call = %+v, want databaseName=mydb targetID=%s", backupCall, target.ID)
	}
	if backupCall.historyID != got.BackupHistoryID {
		t.Errorf("backup call historyID = %q, want the response's own %q", backupCall.historyID, got.BackupHistoryID)
	}

	cloneCall := cloneRunner.awaitCall(t)
	if cloneCall.sourceDatabaseName != "mydb" || cloneCall.newDatabaseName != "mydb-clone" {
		t.Errorf("clone call = %+v, want source=mydb new=mydb-clone", cloneCall)
	}
	if cloneCall.backupHistoryID != got.BackupHistoryID {
		t.Errorf("clone call backupHistoryID = %q, want the same backup id %q", cloneCall.backupHistoryID, got.BackupHistoryID)
	}

	newDB, err := db.GetDesiredDatabase(context.Background(), "mydb-clone")
	if err != nil {
		t.Fatalf("the new database shell was not created: %v", err)
	}
	if newDB.Engine != "postgres" || newDB.Version != "16" {
		t.Errorf("new database = %+v, want engine=postgres version=16 (copied from the source)", newDB)
	}
}

func TestHandleCloneDatabaseNow_ExplicitTargetOverridesSourceDefault(t *testing.T) {
	backupRunner, cloneRunner := newFakeBackupRunner(), newFakeCloneRestoreRunner()
	rt, db := newTestRouterWithCloneNow(t, backupRunner, cloneRunner)
	cookie := loginTestSession(t, rt, db)
	seedDatabaseWithBackupTarget(t, db)

	other := store.BackupTarget{ID: "bkt_other", Name: "secondary", Provider: store.BackupProviderAWS, Bucket: "other", CreatedAt: "2026-08-14T00:00:00Z"}
	if err := db.SaveBackupTarget(context.Background(), other); err != nil {
		t.Fatalf("SaveBackupTarget() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/mydb/clone", `{"new_name":"mydb-clone","target_id":"bkt_other"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	backupCall := backupRunner.awaitCall(t)
	if backupCall.targetID != "bkt_other" {
		t.Errorf("targetID = %q, want the explicitly requested %q, not the source's own default", backupCall.targetID, "bkt_other")
	}
}
