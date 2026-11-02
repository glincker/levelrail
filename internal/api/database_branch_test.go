package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newBranchRouter(t *testing.T, mysql MySQLPITRRestoreRunner) (*Router, *store.DB, *fakeCloneRestoreRunner) {
	t.Helper()
	runner := newFakeCloneRestoreRunner()
	opts := []Option{WithCloneRestoreRunner(runner)}
	if mysql != nil {
		opts = append(opts, WithMySQLPITRRestoreRunner(mysql))
	}
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, opts...), db, runner
}

func postBranch(t *testing.T, rt *Router, cookie *http.Cookie, source, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/"+source+"/branches", body))
	return rec
}

func TestHandleBranchDatabase_FromBackupLeavesSourceUntouched(t *testing.T) {
	t.Parallel()
	rt, db, runner := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	res := &store.ServiceResources{MemoryBytes: 512 << 20}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", Resources: res}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")
	if err := db.SaveServiceDEK(ctx, "main", []byte("dek")); err != nil {
		t.Fatalf("seed dek: %v", err)
	}
	if err := db.SaveSecretValue(ctx, "main", "postgres_password", []byte("source-secret")); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	before, _ := db.GetDesiredDatabase(ctx, "main")

	rec := postBranch(t, rt, cookie, "main", `{"name":"main-try","backup_id":"bkh_1"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	call := runner.awaitCall(t)
	if call.sourceDatabaseName != "main" || call.newDatabaseName != "main-try" || call.backupHistoryID != "bkh_1" || call.engine != store.EnginePostgres {
		t.Errorf("runner call = %+v", call)
	}

	branch, err := db.GetDesiredDatabase(ctx, "main-try")
	if err != nil {
		t.Fatalf("branch not created: %v", err)
	}
	if branch.Engine != before.Engine || branch.Version != before.Version || branch.Resources == nil || branch.Resources.MemoryBytes != res.MemoryBytes {
		t.Errorf("branch = %+v, want source engine, version and size preset", branch)
	}
	if branch.PoolerEnabled {
		t.Error("branch has a pooler enabled")
	}
	after, _ := db.GetDesiredDatabase(ctx, "main")
	if after.Version != before.Version || after.Engine != before.Engine || after.PoolerEnabled != before.PoolerEnabled {
		t.Errorf("source changed: before %+v after %+v", before, after)
	}
	// Credentials: the source secret stays as it was and nothing is copied to the branch.
	if got, _ := db.GetSecretValue(ctx, "main", "postgres_password"); string(got) != "source-secret" {
		t.Errorf("source secret = %q, want unchanged", got)
	}
	if has, _ := db.HasSecretValue(ctx, "main-try", "postgres_password"); has {
		t.Error("branch carries a copied secret, want it generated at provision")
	}
}

func TestHandleBranchDatabase_NameCollision(t *testing.T) {
	t.Parallel()
	rt, db, _ := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, n := range []string{"main", "taken"} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: n, Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	if rec := postBranch(t, rt, cookie, "main", `{"name":"taken","backup_id":"bkh_1"}`); rec.Code != http.StatusConflict {
		t.Errorf("collision status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{`{"name":"main","backup_id":"bkh_1"}`, `{"name":"Bad Name","backup_id":"bkh_1"}`, `{"name":"x"}`, `{"name":"x","backup_id":"bkh_1","at":"2026-01-01T00:00:00Z"}`} {
		if rec := postBranch(t, rt, cookie, "main", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestHandleBranchDatabase_FromPITRTime(t *testing.T) {
	t.Parallel()
	mysql := newFakeMySQLPITRRunner()
	rt, db, _ := newBranchRouter(t, mysql)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedPITRDatabase(t, db, store.EngineMySQL, true)
	tid := seedBackupTargetForAPI(t, db).ID
	for id, at := range map[string]string{"base_old": "2026-08-14T01:00:00Z", "base_new": "2026-08-14T05:00:00Z", "base_late": "2026-08-14T09:00:00Z"} {
		seedBaseBackup(t, db, tid, "main", id, at)
	}

	rec := postBranch(t, rt, cookie, "main", `{"name":"main-pit","at":"2026-08-14T06:00:00Z"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	select {
	case c := <-mysql.calls:
		if c.baseBackupID != "base_new" || c.newName != "main-pit" || c.source != "main" || !c.target.Equal(time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)) {
			t.Errorf("call = %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PITR clone restore not called")
	}
	if _, err := db.GetDesiredDatabase(ctx, "main-pit"); err != nil {
		t.Errorf("branch not created: %v", err)
	}
}

func TestHandleBranchDatabase_PITRRefusals(t *testing.T) {
	t.Parallel()
	rt, db, _ := newBranchRouter(t, newFakeMySQLPITRRunner())
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "pg", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if rec := postBranch(t, rt, cookie, "pg", `{"name":"pg2","at":"2026-08-14T06:00:00Z"}`); rec.Code != http.StatusConflict {
		t.Errorf("postgres at status = %d, want 409", rec.Code)
	}
	seedPITRDatabase(t, db, store.EngineMySQL, false)
	if rec := postBranch(t, rt, cookie, "main", `{"name":"m2","at":"2026-08-14T06:00:00Z"}`); rec.Code != http.StatusConflict {
		t.Errorf("pitr off status = %d, want 409", rec.Code)
	}
	if _, err := db.GetDesiredDatabase(ctx, "pg2"); err == nil {
		t.Error("a refused branch still created a database")
	}
}

func TestBranchRoute_RequiresAuth(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodPost, "/api/v1/databases/main/branches"}})
}

func seedBaseBackup(t *testing.T, db *store.DB, targetID, database, id, finished string) {
	t.Helper()
	ctx := context.Background()
	h := store.BaseBackupHistory{ID: id, DatabaseName: database, TargetID: targetID, ObjectKey: id, StartedAt: finished}
	if err := db.StartBaseBackupHistory(ctx, h); err != nil {
		t.Fatalf("StartBaseBackupHistory: %v", err)
	}
	if err := db.FinishBaseBackupHistory(ctx, id, store.BackupStatusSucceeded, 1, "", "", finished); err != nil {
		t.Fatalf("FinishBaseBackupHistory: %v", err)
	}
}
