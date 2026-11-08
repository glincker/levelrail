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

	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeOneShotEngine stands in for the fresh-container runner: each attempt
// "creates" a container, blocks until done or cancelled, then "removes" it.
type fakeOneShotEngine struct {
	mu      sync.Mutex
	live    int
	removed int
	started chan store.ScheduledTask
	finish  chan struct{}
}

func (f *fakeOneShotEngine) Run(context.Context, store.ScheduledTask) error { return nil }

func (f *fakeOneShotEngine) RunAttempt(ctx context.Context, task store.ScheduledTask) (scheduledtask.AttemptOutcome, error) {
	f.mu.Lock()
	f.live++
	f.mu.Unlock()
	f.started <- task
	defer func() {
		f.mu.Lock()
		f.live--
		f.removed++
		f.mu.Unlock()
	}()
	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	select {
	case <-f.finish:
		return scheduledtask.AttemptOutcome{Status: store.ScheduledTaskStatusSuccess}, nil
	case <-ctx.Done():
		return scheduledtask.AttemptOutcome{Status: store.ScheduledTaskStatusInterrupted}, nil
	case <-time.After(timeout):
		return scheduledtask.AttemptOutcome{Status: store.ScheduledTaskStatusTimeout}, nil
	}
}

func (f *fakeOneShotEngine) counts() (live, removed int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live, f.removed
}

func startOneShot(t *testing.T, rt *Router, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/jobs/runs", body))
	return rec
}

func TestAppsRunAPI_CancelKillsAndRemovesContainerAndAuditsRedacted(t *testing.T) {
	t.Setenv("APP_RUN_MAX_CONCURRENT_PER_APP", "1")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	eng := &fakeOneShotEngine{started: make(chan store.ScheduledTask, 4), finish: make(chan struct{})}
	rt.scheduledTaskRunner = eng

	rec := startOneShot(t, rt, cookie, `{"app":"web","command":["psql","--password","hunter2","DB_TOKEN=abc","postgres://u:pw@db/x"],"backoff_limit":0}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var job oneShotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	<-eng.started

	if rec := startOneShot(t, rt, cookie, `{"app":"web","command":["true"],"backoff_limit":0}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second run past the cap: status = %d, want 429", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/jobs/runs/"+job.ID+"/cancel", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d, body = %s", rec.Code, rec.Body.String())
	}
	waitFor(t, 5*time.Second, func() bool { live, removed := eng.counts(); return live == 0 && removed == 1 })
	waitFor(t, 5*time.Second, func() bool { e, _ := oneShotRegistry.Get(job.ID); return e.Job.Snapshot().State != "Running" })

	entries, err := db.ListAuditEntries(t.Context(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var detail string
	for _, e := range entries {
		if e.Action == auditActionOneShotStart {
			detail = e.Detail
		}
	}
	if detail == "" {
		t.Fatal("no audit entry for the run start")
	}
	for _, leak := range []string{"hunter2", "abc", "pw@"} {
		if strings.Contains(detail, leak) {
			t.Fatalf("audit detail leaks %q: %s", leak, detail)
		}
	}
	if !strings.Contains(detail, "psql --password [redacted]") {
		t.Fatalf("audit detail = %q", detail)
	}

	// The slot is free again once the cancelled run settled.
	waitFor(t, 5*time.Second, func() bool {
		oneShotRuns.mu.Lock()
		defer oneShotRuns.mu.Unlock()
		return oneShotRuns.active["web"] == 0
	})
}

func TestAppsRunAPI_TimeoutIsClampedAndKills(t *testing.T) {
	t.Setenv("APP_RUN_MAX_TIMEOUT_SECONDS", "1")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	eng := &fakeOneShotEngine{started: make(chan store.ScheduledTask, 4), finish: make(chan struct{})}
	rt.scheduledTaskRunner = eng

	if rec := startOneShot(t, rt, cookie, `{"app":"web","command":["sleep","99"],"timeout_seconds":9999,"backoff_limit":0}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	task := <-eng.started
	if task.TimeoutSeconds != 1 {
		t.Fatalf("timeout = %d, want the 1s ceiling", task.TimeoutSeconds)
	}
	waitFor(t, 5*time.Second, func() bool { live, removed := eng.counts(); return live == 0 && removed == 1 })
}

func TestAppsRunAPI_CancelUnknownIs404(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/jobs/runs/jb_none/cancel", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAppsRunRedactArgv(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"key value and secret flag", []string{"env", "API_KEY=x1", "--token", "t1", "--name", "ok"}, "env API_KEY=[redacted] --token [redacted] --name ok"},
		{"auth header after -H", []string{"curl", "-H", "Authorization: Bearer fake1", "https://h"}, "curl -H [redacted] https://h"},
		{"basic auth after -u", []string{"curl", "-u", "user:fake2", "https://h"}, "curl -u [redacted] https://h"},
		{"short password flag", []string{"mysql", "-p", "fake3"}, "mysql -p [redacted]"},
		{"inside sh -c", []string{"sh", "-c", "psql --password fake4 -h db && curl -H \"Authorization: Bearer fake5\" x"}, "sh -c psql --password [redacted] -h db && curl -H \"Authorization: [redacted] x"},
		{"key value inside sh -c", []string{"sh", "-c", "DB_PASSWORD=fake6 ./migrate"}, "sh -c DB_PASSWORD=[redacted] ./migrate"},
		{"url password with slash", []string{"psql", "postgres://u:fa/ke7@db/x"}, "psql postgres://[redacted]@db/x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactRunArgv(tc.argv); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAppsRunAPI_HiddenOneShotTaskNotOnJobsRoutes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	rt.scheduledTaskRunner = &fakeOneShotEngine{started: make(chan store.ScheduledTask, 1), finish: make(chan struct{})}
	now := time.Now().UTC()
	if err := db.SaveScheduledTask(t.Context(), store.ScheduledTask{
		ID: "st_hidden", ServiceName: "web", Command: []string{"psql", "fake-secret-argv"}, Schedule: "@yearly",
		Name: oneShotTaskNamePrefix + "jb_x", Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/jobs", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "fake-secret-argv") {
		t.Fatalf("list: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/apps/web/jobs/st_hidden"},
		{http.MethodGet, "/api/v1/apps/web/jobs/oneshot-jb_x"},
		{http.MethodPost, "/api/v1/apps/web/jobs/st_hidden/run"},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, req.method, req.path, ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s: status = %d, want 404", req.method, req.path, rec.Code)
		}
	}
}
