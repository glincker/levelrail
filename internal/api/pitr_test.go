package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var errTestOutsideWindow = errors.New("target time is outside the recoverable window")

type fakeBaseBackupRunner struct {
	err   error
	calls chan baseBackupRunCall
}

type baseBackupRunCall struct {
	historyID, databaseName, containerName, targetID string
}

func newFakeBaseBackupRunner() *fakeBaseBackupRunner {
	return &fakeBaseBackupRunner{calls: make(chan baseBackupRunCall, 4)}
}

func (f *fakeBaseBackupRunner) RunBaseBackup(_ context.Context, historyID, databaseName, containerName, targetID string) error {
	f.calls <- baseBackupRunCall{historyID, databaseName, containerName, targetID}
	return f.err
}

func (f *fakeBaseBackupRunner) awaitCall(t *testing.T) baseBackupRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunBaseBackup was not called within the deadline")
		return baseBackupRunCall{}
	}
}

type fakePITRRestoreRunner struct {
	hasBaseBackup bool
	windowStart   time.Time
	windowEnd     time.Time
	windowErr     error
	validateErr   error
	runErr        error
	calls         chan pitrRestoreRunCall
}

type pitrRestoreRunCall struct {
	historyID, databaseName, containerName, dataVolumeName, baseBackupHistoryID string
	targetTime                                                                  time.Time
}

func newFakePITRRestoreRunner() *fakePITRRestoreRunner {
	return &fakePITRRestoreRunner{calls: make(chan pitrRestoreRunCall, 4)}
}

func (f *fakePITRRestoreRunner) WindowBounds(_ context.Context, _, _ string) (bool, time.Time, time.Time, error) {
	return f.hasBaseBackup, f.windowStart, f.windowEnd, f.windowErr
}

func (f *fakePITRRestoreRunner) ValidateTarget(context.Context, string, string, time.Time) error {
	return f.validateErr
}

func (f *fakePITRRestoreRunner) RunPITRRestore(_ context.Context, historyID, databaseName, containerName, dataVolumeName, baseBackupHistoryID string, targetTime time.Time) error {
	f.calls <- pitrRestoreRunCall{historyID, databaseName, containerName, dataVolumeName, baseBackupHistoryID, targetTime}
	return f.runErr
}

func (f *fakePITRRestoreRunner) awaitCall(t *testing.T) pitrRestoreRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunPITRRestore was not called within the deadline")
		return pitrRestoreRunCall{}
	}
}

func newTestRouterWithPITR(t *testing.T, baseBackup BaseBackupRunner, pitrRestore PITRRestoreRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	opts := []Option{}
	if baseBackup != nil {
		opts = append(opts, WithBaseBackupRunner(baseBackup))
	}
	if pitrRestore != nil {
		opts = append(opts, WithPITRRestoreRunner(pitrRestore))
	}
	return NewRouter(logger, testBrand(), db, opts...), db
}

func seedPostgresDatabaseForAPI(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
}

func TestPITRRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/databases/main/pitr"},
		{http.MethodDelete, "/api/v1/databases/main/pitr"},
		{http.MethodGet, "/api/v1/databases/main/pitr"},
		{http.MethodPost, "/api/v1/databases/main/base-backups"},
		{http.MethodGet, "/api/v1/databases/main/base-backups"},
		{http.MethodPost, "/api/v1/databases/main/pitr-restore"},
		{http.MethodGet, "/api/v1/databases/main/pitr-restores"},
	})
}

func TestHandleEnablePITR_RejectsNonPostgres(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedRedisDatabaseForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleEnablePITR_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/missing/pitr", ``))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleEnableThenDisablePITR_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if !got.PITREnabled || got.PITREnabledAt == "" {
		t.Fatalf("after enable: PITREnabled=%v PITREnabledAt=%q, want true and non-empty", got.PITREnabled, got.PITREnabledAt)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	got, err = db.GetDesiredDatabase(context.Background(), "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.PITREnabled {
		t.Fatal("after disable: PITREnabled = true, want false")
	}
}

func TestHandleGetPITRStatus_NotEnabled(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got pitrStatusResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Error("Enabled = true, want false")
	}
}

func TestHandleGetPITRStatus_EnabledWithWindow(t *testing.T) {
	runner := newFakePITRRestoreRunner()
	start := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 14, 6, 0, 0, 0, time.UTC)
	runner.hasBaseBackup = true
	runner.windowStart = start
	runner.windowEnd = end

	rt, db := newTestRouterWithPITR(t, nil, runner)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	if err := db.SetDatabasePITR(context.Background(), "main", true, "2026-08-14T00:00:00Z"); err != nil {
		t.Fatalf("SetDatabasePITR() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got pitrStatusResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled || !got.HasBaseBackup {
		t.Fatalf("got = %+v, want Enabled and HasBaseBackup true", got)
	}
	if got.WindowStart != start.Format(time.RFC3339) || got.WindowEnd != end.Format(time.RFC3339) {
		t.Errorf("window = [%s, %s], want [%s, %s]", got.WindowStart, got.WindowEnd, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
}

func TestHandleTriggerBaseBackup_RequiresPITREnabled(t *testing.T) {
	runner := newFakeBaseBackupRunner()
	rt, db := newTestRouterWithPITR(t, runner, nil)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	seedBackupTargetForAPI(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/base-backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleTriggerBaseBackup_Success(t *testing.T) {
	runner := newFakeBaseBackupRunner()
	rt, db := newTestRouterWithPITR(t, runner, nil)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	seedBackupTargetForAPI(t, db)
	if err := db.SetDatabasePITR(context.Background(), "main", true, "2026-08-14T00:00:00Z"); err != nil {
		t.Fatalf("SetDatabasePITR() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/base-backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	call := runner.awaitCall(t)
	if call.databaseName != "main" || call.containerName != "db-main" || call.targetID != "bkt_test1" {
		t.Errorf("call = %+v, want database=main container=db-main target=bkt_test1", call)
	}
}

func TestHandleListBaseBackupHistory_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/base-backups", ``))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerPITRRestore_NotEnabled(t *testing.T) {
	runner := newFakePITRRestoreRunner()
	rt, db := newTestRouterWithPITR(t, nil, runner)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)

	body := `{"base_backup_id":"bbh_1","target_time":"2026-08-14T03:00:00Z"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleTriggerPITRRestore_InvalidTimestamp(t *testing.T) {
	runner := newFakePITRRestoreRunner()
	rt, db := newTestRouterWithPITR(t, nil, runner)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	if err := db.SetDatabasePITR(context.Background(), "main", true, "2026-08-14T00:00:00Z"); err != nil {
		t.Fatalf("SetDatabasePITR() error = %v", err)
	}

	body := `{"base_backup_id":"bbh_1","target_time":"not-a-timestamp"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleTriggerPITRRestore_OutsideWindow_Rejected(t *testing.T) {
	runner := newFakePITRRestoreRunner()
	runner.validateErr = errTestOutsideWindow
	rt, db := newTestRouterWithPITR(t, nil, runner)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	if err := db.SetDatabasePITR(context.Background(), "main", true, "2026-08-14T00:00:00Z"); err != nil {
		t.Fatalf("SetDatabasePITR() error = %v", err)
	}

	body := `{"base_backup_id":"bbh_1","target_time":"2026-08-14T03:00:00Z"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	select {
	case <-runner.calls:
		t.Fatal("RunPITRRestore must never be called when ValidateTarget rejects the request")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHandleTriggerPITRRestore_Success(t *testing.T) {
	runner := newFakePITRRestoreRunner()
	rt, db := newTestRouterWithPITR(t, nil, runner)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	if err := db.SetDatabasePITR(context.Background(), "main", true, "2026-08-14T00:00:00Z"); err != nil {
		t.Fatalf("SetDatabasePITR() error = %v", err)
	}

	target := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	body := `{"base_backup_id":"bbh_1","target_time":"` + target.Format(time.RFC3339) + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr-restore", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got pitrRestoreHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != store.BackupStatusRunning || got.BaseBackupHistoryID != "bbh_1" {
		t.Errorf("got = %+v, want status=running base_backup_id=bbh_1", got)
	}

	call := runner.awaitCall(t)
	if call.databaseName != "main" || call.containerName != "db-main" || call.dataVolumeName != "db-main-data" {
		t.Errorf("call = %+v, want database=main container=db-main volume=db-main-data", call)
	}
	if !call.targetTime.Equal(target) {
		t.Errorf("call.targetTime = %v, want %v", call.targetTime, target)
	}
}

func TestHandleListPITRRestoreHistory_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/pitr-restores", ``))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
