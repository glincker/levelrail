package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduledTaskRunner is a hand-written fake for ScheduledTaskRunner,
// the same buffered-channel pattern fakeBackupRunner (backups_test.go)
// already establishes for handleTriggerBackup's own detached goroutine.
type fakeScheduledTaskRunner struct {
	err   error
	calls chan scheduledTaskRunCall
}

type scheduledTaskRunCall struct {
	runID string
	task  store.ScheduledTask
}

func newFakeScheduledTaskRunner() *fakeScheduledTaskRunner {
	return &fakeScheduledTaskRunner{calls: make(chan scheduledTaskRunCall, 4)}
}

func (f *fakeScheduledTaskRunner) Run(_ context.Context, runID string, task store.ScheduledTask) error {
	f.calls <- scheduledTaskRunCall{runID: runID, task: task}
	return f.err
}

func (f *fakeScheduledTaskRunner) awaitCall(t *testing.T) scheduledTaskRunCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("Run was not called within the deadline")
		return scheduledTaskRunCall{}
	}
}

func newTestRouterWithScheduledTaskRunner(t *testing.T, runner ScheduledTaskRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithScheduledTaskRunner(runner)), db
}

func seedScheduledTaskApp(t *testing.T, db *store.DB) {
	t.Helper()
	svc := store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("seed app: %v", err)
	}
}

const validScheduledTaskBody = `{"name":"cron","command":"php artisan schedule:run","schedule":"* * * * *","enabled":true}`

func TestHandleCreateScheduledTask_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", validScheduledTaskBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a minted id")
	}
	if got.ServiceName != "web" || got.Command != "php artisan schedule:run" || got.Schedule != "* * * * *" || !got.Enabled {
		t.Errorf("got = %+v, unexpected fields", got)
	}
}

func TestHandleCreateScheduledTask_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/scheduled-tasks", validScheduledTaskBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleCreateScheduledTask_InvalidSchedule(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"name":"cron","command":"true","schedule":"not a cron string","enabled":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateScheduledTask_EmptyCommand(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"name":"cron","command":"  ","schedule":"* * * * *","enabled":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandleCreateScheduledTask_PlainWriteToken_Forbidden proves this
// route sits behind AbilityRoot, not AbilityWrite: defining an arbitrary
// command a container will run is the same capability class as exec
// itself.
func TestHandleCreateScheduledTask_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedScheduledTaskApp(t, db)

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/scheduled-tasks", strings.NewReader(validScheduledTaskBody))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to define a scheduled task", rec.Code, http.StatusForbidden)
	}
}

func TestHandleListScheduledTasks_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	createRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", validScheduledTaskBody))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("seed create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/scheduled-tasks", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func createTestScheduledTask(t *testing.T, rt *Router, cookie *http.Cookie) scheduledTaskResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", validScheduledTaskBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestHandleUpdateScheduledTask_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)
	task := createTestScheduledTask(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/scheduled-tasks/"+task.ID,
		`{"name":"cron updated","command":"true","schedule":"*/5 * * * *","enabled":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "cron updated" || got.Enabled {
		t.Errorf("got = %+v, want updated name and enabled=false", got)
	}
}

func TestHandleUpdateScheduledTask_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/scheduled-tasks/sct_nonexistent", validScheduledTaskBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleUpdateScheduledTask_WrongApp_NotFound proves a task ID that
// belongs to a different app is treated as not-found through this app's
// own URL, not exposed cross-app.
func TestHandleUpdateScheduledTask_WrongApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "other", Image: "levelrail/other:1", Port: 4000}); err != nil {
		t.Fatalf("seed other app: %v", err)
	}
	task := createTestScheduledTask(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/other/scheduled-tasks/"+task.ID, validScheduledTaskBody))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleDeleteScheduledTask_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)
	task := createTestScheduledTask(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/scheduled-tasks/"+task.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/scheduled-tasks", ""))
	var got []scheduledTaskResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d after delete, want 0", len(got))
	}
}

func TestHandleDeleteScheduledTask_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/scheduled-tasks/sct_nonexistent", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleRunScheduledTask_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithScheduledTaskRunner
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)
	task := createTestScheduledTask(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks/"+task.ID+"/run", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleRunScheduledTask_Success(t *testing.T) {
	runner := newFakeScheduledTaskRunner()
	rt, db := newTestRouterWithScheduledTaskRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)
	task := createTestScheduledTask(t, rt, cookie)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks/"+task.ID+"/run", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	call := runner.awaitCall(t)
	if call.task.ID != task.ID || call.task.ServiceName != "web" {
		t.Errorf("Run called with task = %+v, want ID=%s ServiceName=web", call.task, task.ID)
	}
	if call.runID == "" {
		t.Error("runID is empty")
	}
}

func TestHandleRunScheduledTask_NotFound(t *testing.T) {
	runner := newFakeScheduledTaskRunner()
	rt, db := newTestRouterWithScheduledTaskRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks/sct_nonexistent/run", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListScheduledTaskRuns_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)
	task := createTestScheduledTask(t, rt, cookie)

	if err := db.StartScheduledTaskRun(context.Background(), store.ScheduledTaskRun{ID: "sctr_1", ScheduledTaskID: task.ID, StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := db.FinishScheduledTaskRun(context.Background(), "sctr_1", store.ScheduledTaskRunStatusSucceeded, 0, "ok\n", "", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/scheduled-tasks/"+task.ID+"/runs", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []scheduledTaskRunResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Status != store.ScheduledTaskRunStatusSucceeded || got[0].Output != "ok\n" {
		t.Errorf("got = %+v, want one succeeded run with output %q", got, "ok\n")
	}
}

func TestHandleListScheduledTaskRuns_TaskNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/scheduled-tasks/sct_nonexistent/runs", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
