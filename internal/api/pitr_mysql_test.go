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

type fakeMySQLPITRRunner struct {
	start, end  time.Time
	validateErr error
	calls       chan mysqlPITRCall
}

type mysqlPITRCall struct {
	historyID, source, newName, baseBackupID, engine, sourceContainer, newContainer, newController string
	target                                                                                         time.Time
}

func newFakeMySQLPITRRunner() *fakeMySQLPITRRunner {
	return &fakeMySQLPITRRunner{calls: make(chan mysqlPITRCall, 2)}
}

func (f *fakeMySQLPITRRunner) WindowBounds(context.Context, string, string) (bool, time.Time, time.Time, error) {
	return true, f.start, f.end, nil
}

func (f *fakeMySQLPITRRunner) ValidateTarget(context.Context, string, string, time.Time) error {
	return f.validateErr
}

func (f *fakeMySQLPITRRunner) RunPITRCloneRestore(_ context.Context, historyID, source, newName, baseBackupID, engine, sourceContainer, newContainer, newController string, target time.Time) error {
	f.calls <- mysqlPITRCall{historyID, source, newName, baseBackupID, engine, sourceContainer, newContainer, newController, target}
	return nil
}

func newMySQLPITRRouter(t *testing.T, runner MySQLPITRRestoreRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	opts := []Option{}
	if runner != nil {
		opts = append(opts, WithMySQLPITRRestoreRunner(runner))
	}
	return NewRouter(logger, testBrand(), db, opts...), db
}

func seedPITRDatabase(t *testing.T, db *store.DB, engine string, enabled bool) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: engine, Version: "8.4"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if enabled {
		if err := db.SetDatabasePITR(context.Background(), "main", true, "2026-08-14T00:00:00Z"); err != nil {
			t.Fatalf("SetDatabasePITR() error = %v", err)
		}
	}
}

func TestHandleEnablePITR_AcceptsMySQLAndMariaDB(t *testing.T) {
	for _, engine := range []string{store.EngineMySQL, store.EngineMariaDB} {
		t.Run(engine, func(t *testing.T) {
			rt, db := newMySQLPITRRouter(t, nil)
			cookie := loginTestSession(t, rt, db)
			seedPITRDatabase(t, db, engine, false)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr", ``))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
			}
			d, err := db.GetDesiredDatabase(context.Background(), "main")
			if err != nil || !d.PITREnabled {
				t.Fatalf("PITREnabled = %v err = %v, want true", d != nil && d.PITREnabled, err)
			}
		})
	}
}

func TestHandleEnablePITR_StillRejectsMongoDB(t *testing.T) {
	rt, db := newMySQLPITRRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	seedPITRDatabase(t, db, store.EngineMongoDB, false)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetPITRStatus_MySQLReportsNewDatabaseRestoreMode(t *testing.T) {
	runner := newFakeMySQLPITRRunner()
	runner.start = time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	runner.end = time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)
	rt, db := newMySQLPITRRouter(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedPITRDatabase(t, db, store.EngineMySQL, true)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/pitr", ``))
	var got pitrStatusResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RestoreMode != pitrRestoreModeNewDatabase || got.Engine != store.EngineMySQL || !got.HasBaseBackup || got.WindowEnd != "2026-08-14T06:00:00Z" {
		t.Errorf("got = %+v", got)
	}
}

func TestHandleTriggerPITRRestore_MySQLRequiresRestoreAs(t *testing.T) {
	runner := newFakeMySQLPITRRunner()
	rt, db := newMySQLPITRRouter(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedPITRDatabase(t, db, store.EngineMySQL, true)

	body := `{"base_backup_id":"bbh_1","target_time":"2026-08-14T03:00:00Z"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	body = `{"base_backup_id":"bbh_1","target_time":"2026-08-14T03:00:00Z","restore_as":"main"}`
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("restoring over the source: status = %d, want 400", rec.Code)
	}
}

func TestHandleTriggerPITRRestore_MySQLRejectsUnsafeRestoreAs(t *testing.T) {
	for _, name := range []string{"x$(id)", "a`id`", "Copy", "a;b", "a b", "-lead", "has_underscore", strings.Repeat("a", 64)} {
		runner := newFakeMySQLPITRRunner()
		rt, db := newMySQLPITRRouter(t, runner)
		cookie := loginTestSession(t, rt, db)
		seedPITRDatabase(t, db, store.EngineMySQL, true)

		payload, err := json.Marshal(map[string]string{"base_backup_id": "bbh_1", "target_time": "2026-08-14T03:00:00Z", "restore_as": name})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", string(payload)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("restore_as %q: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestHandleTriggerPITRRestore_MySQLOutsideWindowCreatesNothing(t *testing.T) {
	runner := newFakeMySQLPITRRunner()
	runner.validateErr = errTestOutsideWindow
	rt, db := newMySQLPITRRouter(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedPITRDatabase(t, db, store.EngineMySQL, true)

	body := `{"base_backup_id":"bbh_1","target_time":"2026-08-14T03:00:00Z","restore_as":"copy"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "copy"); err == nil {
		t.Error("a rejected restore still created the new database")
	}
}

func TestHandleTriggerPITRRestore_MySQLCreatesNewDatabaseWithTLSAndRunsRestore(t *testing.T) {
	runner := newFakeMySQLPITRRunner()
	rt, db := newMySQLPITRRouter(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedPITRDatabase(t, db, store.EngineMariaDB, true)

	target := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	body := `{"base_backup_id":"bbh_1","target_time":"` + target.Format(time.RFC3339) + `","restore_as":"copy"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", rec.Code, rec.Body.String())
	}

	select {
	case c := <-runner.calls:
		if c.source != "main" || c.newName != "copy" || c.engine != store.EngineMariaDB || c.newContainer != "db-copy" ||
			c.sourceContainer != "db-main" || c.newController != "database/copy" || !c.target.Equal(target) || c.baseBackupID != "bbh_1" {
			t.Errorf("call = %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunPITRCloneRestore was not called")
	}

	d, err := db.GetDesiredDatabase(context.Background(), "copy")
	if err != nil || d.Engine != store.EngineMariaDB {
		t.Fatalf("new database = %+v err = %v", d, err)
	}
	if mode, _ := db.GetDatabaseTLSMode(context.Background(), "copy"); mode != store.DatabaseTLSModeOn {
		t.Errorf("new database tls mode = %q, want on", mode)
	}
	if mode, _ := db.GetDatabaseTLSMode(context.Background(), "main"); mode != store.DatabaseTLSModeDefault {
		t.Errorf("source database tls mode = %q, it must stay default", mode)
	}
}
