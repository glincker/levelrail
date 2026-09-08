package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeUploadRestoreRunner is a hand-written fake for RestoreRunner,
// exercising only RunRestoreFromReader: the restore-upload handler never
// calls RunRestore, so that method just satisfies the interface.
type fakeUploadRestoreRunner struct {
	err   error
	calls chan uploadRestoreRunCall
}

type uploadRestoreRunCall struct {
	historyID, databaseName, engine, containerName, body string
}

func newFakeUploadRestoreRunner() *fakeUploadRestoreRunner {
	return &fakeUploadRestoreRunner{calls: make(chan uploadRestoreRunCall, 4)}
}

func (f *fakeUploadRestoreRunner) RunRestore(_ context.Context, _, _, _, _, _ string) error {
	return errors.New("fakeUploadRestoreRunner: RunRestore should not be called by the upload handler")
}

func (f *fakeUploadRestoreRunner) RunRestoreFromReader(_ context.Context, historyID, databaseName, engine, containerName string, dump io.Reader) error {
	body, _ := io.ReadAll(dump)
	f.calls <- uploadRestoreRunCall{historyID, databaseName, engine, containerName, string(body)}
	return f.err
}

func (f *fakeUploadRestoreRunner) awaitCall(t *testing.T) uploadRestoreRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("RunRestoreFromReader was not called within the deadline")
		return uploadRestoreRunCall{}
	}
}

func newTestRouterWithUploadRestoreRunner(t *testing.T, runner RestoreRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRestoreRunner(runner)), db
}

func TestHandleTriggerRestoreUpload_NoRunnerConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithRestoreRunner
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-upload", "dump-bytes"))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleTriggerRestoreUpload_DatabaseNotFound(t *testing.T) {
	runner := newFakeUploadRestoreRunner()
	rt, db := newTestRouterWithUploadRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/missing/restore-upload", "dump-bytes"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerRestoreUpload_Success(t *testing.T) {
	runner := newFakeUploadRestoreRunner()
	rt, db := newTestRouterWithUploadRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-upload", "raw-dump-bytes"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got restoreHistoryResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(got.ID, "rsh_") {
		t.Errorf("ID = %q, want an rsh_-prefixed id", got.ID)
	}
	if got.DatabaseName != "main" || got.Status != store.BackupStatusSucceeded {
		t.Errorf("response = %+v, want database_name=main status=succeeded", got)
	}
	if got.BackupHistoryID != "" {
		t.Errorf("BackupHistoryID = %q, want empty for an uploaded-file restore", got.BackupHistoryID)
	}

	call := runner.awaitCall(t)
	if call.databaseName != "main" || call.engine != store.EnginePostgres || call.containerName != "db-main" {
		t.Errorf("RunRestoreFromReader call = %+v, want database=main engine=postgres container=db-main", call)
	}
	if call.body != "raw-dump-bytes" {
		t.Errorf("RunRestoreFromReader body = %q, want %q", call.body, "raw-dump-bytes")
	}
	if call.historyID != got.ID {
		t.Errorf("RunRestoreFromReader historyID = %q, want it to match the response id %q", call.historyID, got.ID)
	}
}

func TestHandleTriggerRestoreUpload_RunnerFails(t *testing.T) {
	runner := newFakeUploadRestoreRunner()
	runner.err = errors.New("psql: syntax error")
	rt, db := newTestRouterWithUploadRestoreRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restore-upload", "bad-dump"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "psql: syntax error") {
		t.Errorf("body = %q, want it to surface the underlying restore failure", rec.Body.String())
	}
}

func TestRestoreUploadRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/main/restore-upload", strings.NewReader("dump"))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
