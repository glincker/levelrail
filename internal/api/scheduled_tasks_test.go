package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeScheduledTaskRunner is Router's own fake for ScheduledTaskRunner:
// records every call and signals completion through a channel, since
// handleRunScheduledTaskNow dispatches into its own goroutine (matching
// handleTriggerBackup's own async shape).
type fakeScheduledTaskRunner struct {
	mu    sync.Mutex
	calls []store.ScheduledTask
	done  chan struct{}
	err   error
}

func newFakeScheduledTaskRunner() *fakeScheduledTaskRunner {
	return &fakeScheduledTaskRunner{done: make(chan struct{}, 8)}
}

func (f *fakeScheduledTaskRunner) Run(_ context.Context, task store.ScheduledTask) error {
	f.mu.Lock()
	f.calls = append(f.calls, task)
	f.mu.Unlock()
	f.done <- struct{}{}
	return f.err
}

func (f *fakeScheduledTaskRunner) waitForCall(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the scheduled task runner to be called")
	}
}

func newTestRouterWithScheduledTaskRunner(t *testing.T, runner ScheduledTaskRunner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithScheduledTaskRunner(runner)), db
}

func seedScheduledTaskApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app %q: %v", name, err)
	}
}

func TestHandleCreateScheduledTask_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"command":["sh","-c","echo hi"],"schedule":"0 3 * * *","enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" || got.ServiceName != "web" || got.Schedule != "0 3 * * *" || !got.Enabled {
		t.Errorf("created resource = %+v, unexpected", got)
	}
	if len(got.Command) != 3 || got.Command[2] != "echo hi" {
		t.Errorf("Command = %v, want [sh -c \"echo hi\"]", got.Command)
	}
}

func TestHandleCreateScheduledTask_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/scheduled-tasks", body))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleCreateScheduledTask_EmptyCommand_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"command":[],"schedule":"0 3 * * *","enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateScheduledTask_InvalidSchedule_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	body := `{"command":["echo","hi"],"schedule":"not a cron expr","enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleListScheduledTasks_ScopedToApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "worker")

	create := func(app string) {
		rec := httptest.NewRecorder()
		body := `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/scheduled-tasks", body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create for %q status = %d, body = %s", app, rec.Code, rec.Body.String())
		}
	}
	create("web")
	create("web")
	create("worker")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/scheduled-tasks", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("list = %d tasks, want 2 (scoped to web only)", len(got))
	}
}

func TestHandleGetScheduledTask_OtherAppOwned_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "worker")

	rec := httptest.NewRecorder()
	body := `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", body))
	var created scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/worker/scheduled-tasks/"+created.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d: a task belonging to a different app must not be reachable through it", rec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateScheduledTask_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`))
	var created scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	updateBody := `{"command":["echo","bye"],"schedule":"*/5 * * * *","enabled":false}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/scheduled-tasks/"+created.ID, updateBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Schedule != "*/5 * * * *" || got.Enabled {
		t.Errorf("updated resource = %+v, want schedule=*/5 * * * * enabled=false", got)
	}
}

func TestHandleUpdateScheduledTask_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/scheduled-tasks/sct_missing", `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleDeleteScheduledTask_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`))
	var created scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/scheduled-tasks/"+created.ID, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/scheduled-tasks/"+created.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status after delete = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleRunScheduledTaskNow_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithScheduledTaskRunner
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`))
	var created scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks/"+created.ID+"/run", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleRunScheduledTaskNow_DispatchesToRunner(t *testing.T) {
	runner := newFakeScheduledTaskRunner()
	rt, db := newTestRouterWithScheduledTaskRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`))
	var created scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks/"+created.ID+"/run", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	runner.waitForCall(t)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 1 || runner.calls[0].ID != created.ID {
		t.Errorf("Run calls = %+v, want exactly one call for %q", runner.calls, created.ID)
	}
}

func TestHandleRunScheduledTaskNow_UnknownTask_NotFound(t *testing.T) {
	runner := newFakeScheduledTaskRunner()
	rt, db := newTestRouterWithScheduledTaskRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks/sct_missing/run", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestScheduledTaskRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/apps/web/scheduled-tasks"},
		{http.MethodGet, "/api/v1/apps/web/scheduled-tasks"},
		{http.MethodGet, "/api/v1/apps/web/scheduled-tasks/sct_1"},
		{http.MethodPut, "/api/v1/apps/web/scheduled-tasks/sct_1"},
		{http.MethodDelete, "/api/v1/apps/web/scheduled-tasks/sct_1"},
		{http.MethodPost, "/api/v1/apps/web/scheduled-tasks/sct_1/run"},
	}
	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want %d", route.method, route.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

// TestHandleRunScheduledTaskNow_PlainWriteToken_Forbidden proves run-now
// sits behind AbilityDeploy, not AbilityWrite: a token scoped only to
// AbilityWrite can manage scheduled tasks (create/update/delete) but
// must not be able to actually trigger one running.
func TestHandleRunScheduledTaskNow_PlainWriteToken_Forbidden(t *testing.T) {
	runner := newFakeScheduledTaskRunner()
	rt, db := newTestRouterWithScheduledTaskRunner(t, runner)
	ctx := context.Background()
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"command":["echo","hi"],"schedule":"0 3 * * *","enabled":true}`))
	var created scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/scheduled-tasks/"+created.ID+"/run", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to trigger a scheduled task run", rec.Code, http.StatusForbidden)
	}
	if len(runner.calls) != 0 {
		t.Errorf("Run called %d times, want 0: the ability check must reject before dispatch", len(runner.calls))
	}
}
