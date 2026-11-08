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

// consoleExecJobRoutes are the job routes that set or run a command, so
// they follow the console exec tier (AbilityRoot plus the app's exec setting).
var consoleExecJobRoutes = []string{
	"POST /api/v1/jobs/runs",
	"POST /api/v1/apps/{name}/jobs",
	"PUT /api/v1/apps/{name}/jobs/{id}",
	"POST /api/v1/apps/{name}/jobs/{id}/run",
	"POST /api/v1/apps/{name}/jobs/{id}/enable",
	"POST /api/v1/apps/{name}/scheduled-tasks",
	"PUT /api/v1/apps/{name}/scheduled-tasks/{id}",
	"POST /api/v1/apps/{name}/scheduled-tasks/{id}/run",
	"POST /api/v1/apps/{name}/scheduled-tasks/{id}/enable",
}

func TestAuthzMatrix_JobCommandRoutesFollowConsoleExecTier(t *testing.T) {
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	rt, db, _ := newPipelineRouter(t)
	tok := seedMatrixToken(t, db, "nonroot", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy})
	for _, key := range consoleExecJobRoutes {
		if !registered[key] {
			t.Errorf("%s is not registered in a routes*.go file", key)
			continue
		}
		method, pattern, _ := strings.Cut(key, " ")
		if got := matrixDo(rt, method, concretePath(pattern, "web"), bearer(tok)); got != http.StatusForbidden {
			t.Errorf("%s with every non-root ability -> %d, want 403", key, got)
		}
	}
}

type jobAccessFixture struct {
	rt     *Router
	db     *store.DB
	runner *fakeScheduledTaskRunner
	taskID string
}

func newJobAccessFixture(t *testing.T) jobAccessFixture {
	t.Helper()
	runner := newFakeScheduledTaskRunner()
	rt, db := newTestRouterWithScheduledTaskRunner(t, runner)
	seedScheduledTaskApp(t, db, "web")
	now := time.Now().UTC()
	task := store.ScheduledTask{
		ID: "sct_fixture", ServiceName: "web", Command: []string{"echo", "hi"}, Schedule: "0 3 * * *",
		Enabled: true, Name: "nightly", Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.SaveScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return jobAccessFixture{rt: rt, db: db, runner: runner, taskID: task.ID}
}

type jobAccessCall struct{ name, method, path, body string }

func (f jobAccessFixture) calls() []jobAccessCall {
	return []jobAccessCall{
		{"start one-off", http.MethodPost, "/api/v1/jobs/runs", `{"app":"web","command":["echo","hi"]}`},
		{"create", http.MethodPost, "/api/v1/apps/web/jobs", `{"command":["echo","hi"],"schedule":"0 4 * * *"}`},
		{"update", http.MethodPut, "/api/v1/apps/web/jobs/" + f.taskID, `{"command":["echo","bye"]}`},
		{"run now", http.MethodPost, "/api/v1/apps/web/jobs/" + f.taskID + "/run", ""},
		{"enable", http.MethodPost, "/api/v1/apps/web/jobs/" + f.taskID + "/enable", ""},
	}
}

func (f jobAccessFixture) disableCall() jobAccessCall {
	return jobAccessCall{"disable", http.MethodPost, "/api/v1/apps/web/jobs/" + f.taskID + "/disable", ""}
}

func (f jobAccessFixture) do(token string, c jobAccessCall) *httptest.ResponseRecorder {
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestJobCommandRoutes_NonRootTokensForbidden(t *testing.T) {
	f := newJobAccessFixture(t)
	tokens := map[string][]string{
		"deploy only":    {AbilityDeploy},
		"write only":     {AbilityWrite},
		"write+deploy":   {AbilityWrite, AbilityDeploy},
		"read sensitive": {AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy},
	}
	for label, abilities := range tokens {
		tok := seedMatrixToken(t, f.db, strings.ReplaceAll(label, " ", "_"), abilities)
		for _, c := range f.calls() {
			if rec := f.do(tok, c); rec.Code != http.StatusForbidden {
				t.Errorf("%s token, %s: status %d, want 403 (%s)", label, c.name, rec.Code, rec.Body.String())
			}
		}
	}
	if n := len(f.runner.calls); n != 0 {
		t.Errorf("runner called %d times, want 0", n)
	}
	writer := seedMatrixToken(t, f.db, "writer_disable", []string{AbilityWrite})
	if rec := f.do(writer, f.disableCall()); rec.Code != http.StatusOK {
		t.Errorf("write token disable: status %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestJobCommandRoutes_RootAllowed(t *testing.T) {
	f := newJobAccessFixture(t)
	tok := seedMatrixToken(t, f.db, "root", []string{AbilityRoot})
	want := map[string]int{
		// The fake runner cannot run attempts, so a permitted one-off start answers 501.
		"start one-off": http.StatusNotImplemented,
		"create":        http.StatusCreated,
		"update":        http.StatusOK,
		"run now":       http.StatusAccepted,
		"enable":        http.StatusOK,
	}
	for _, c := range f.calls() {
		if rec := f.do(tok, c); rec.Code != want[c.name] {
			t.Errorf("root, %s: status %d, want %d (%s)", c.name, rec.Code, want[c.name], rec.Body.String())
		}
	}
}

func TestJobCommandRoutes_ExecDisabledRefusesRoot(t *testing.T) {
	f := newJobAccessFixture(t)
	if err := f.db.SetServiceExecEnabled(context.Background(), "web", false); err != nil {
		t.Fatalf("disable exec: %v", err)
	}
	tok := seedMatrixToken(t, f.db, "root", []string{AbilityRoot})
	for _, c := range f.calls() {
		rec := f.do(tok, c)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "console exec access") {
			t.Errorf("exec off, %s: status %d body %s, want 403 naming the access rule", c.name, rec.Code, rec.Body.String())
		}
	}
	if n := len(f.runner.calls); n != 0 {
		t.Errorf("runner called %d times, want 0", n)
	}
	if rec := f.do(tok, f.disableCall()); rec.Code != http.StatusOK {
		t.Errorf("exec off, disable: status %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestJobRoutes_OlderPrefixedUserJobStaysVisible(t *testing.T) {
	f := newJobAccessFixture(t)
	now := time.Now().UTC()
	user := store.ScheduledTask{
		ID: "sct_user", ServiceName: "web", Command: []string{"echo", "x"}, Schedule: "0 1 * * *",
		Name: oneShotTaskNamePrefix + "legacy", Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := f.db.SaveScheduledTask(context.Background(), user); err != nil {
		t.Fatalf("seed user task: %v", err)
	}
	tok := seedMatrixToken(t, f.db, "root", []string{AbilityRoot})
	if rec := f.do(tok, jobAccessCall{"get", http.MethodGet, "/api/v1/apps/web/jobs/" + user.ID, ""}); rec.Code != http.StatusOK {
		t.Errorf("older user job: status %d, want 200", rec.Code)
	}
}

func TestJobRoutes_HideOneShotTasks(t *testing.T) {
	f := newJobAccessFixture(t)
	now := time.Now().UTC()
	hidden := store.ScheduledTask{
		ID: "sct_hidden", ServiceName: "web", Command: []string{"echo", "x"}, Schedule: "@yearly",
		Name: oneShotTaskNamePrefix + "job1", Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := f.db.SaveScheduledTask(context.Background(), hidden); err != nil {
		t.Fatalf("seed hidden task: %v", err)
	}
	tok := seedMatrixToken(t, f.db, "root", []string{AbilityRoot})

	rec := f.do(tok, jobAccessCall{"list", http.MethodGet, "/api/v1/apps/web/jobs", ""})
	var listed []scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v (%s)", err, rec.Body.String())
	}
	for _, j := range listed {
		if j.ID == hidden.ID {
			t.Errorf("list returned the hidden one-off task %+v", j)
		}
	}
	for _, c := range []jobAccessCall{
		{"get", http.MethodGet, "/api/v1/apps/web/jobs/" + hidden.ID, ""},
		{"get by name", http.MethodGet, "/api/v1/apps/web/jobs/" + hidden.Name, ""},
		{"runs", http.MethodGet, "/api/v1/apps/web/jobs/" + hidden.ID + "/runs", ""},
		{"run now", http.MethodPost, "/api/v1/apps/web/jobs/" + hidden.ID + "/run", ""},
		{"enable", http.MethodPost, "/api/v1/apps/web/jobs/" + hidden.ID + "/enable", ""},
	} {
		if rec := f.do(tok, c); rec.Code != http.StatusNotFound {
			t.Errorf("%s on hidden task: status %d, want 404", c.name, rec.Code)
		}
	}
	reserved := jobAccessCall{"create reserved", http.MethodPost, "/api/v1/apps/web/jobs", `{"name":"oneshot-mine","command":["echo","hi"],"schedule":"0 4 * * *"}`}
	if rec := f.do(tok, reserved); rec.Code != http.StatusBadRequest {
		t.Errorf("reserved job name: status %d, want 400", rec.Code)
	}
}
