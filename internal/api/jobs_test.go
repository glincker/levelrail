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

type fakeJobStarter struct {
	tasks    []store.ScheduledTask
	triggers []string
}

func (f *fakeJobStarter) Run(context.Context, store.ScheduledTask) error { return nil }

func (f *fakeJobStarter) Start(_ context.Context, task store.ScheduledTask, trigger string) (string, error) {
	f.tasks = append(f.tasks, task)
	f.triggers = append(f.triggers, trigger)
	return "jr_test", nil
}

func doJobs(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func createJob(t *testing.T, rt *Router, cookie *http.Cookie, body string) scheduledTaskResource {
	t.Helper()
	rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got scheduledTaskResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestJobs_CreateDefaultsAndNextRun(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	got := createJob(t, rt, cookie, `{"name":"nightly","command":["sh","-c","echo hi"],"schedule":"0 3 * * *","timezone":"Asia/Kolkata"}`)
	if got.Mode != store.ScheduledTaskModeFresh || !got.Enabled || got.Timezone != "Asia/Kolkata" || got.Source != store.ScheduledTaskSourceAPI {
		t.Errorf("created = %+v, want fresh mode, enabled, Asia/Kolkata, api source", got)
	}
	if got.NextRunAt == nil || got.NextRunAt.Minute() != 30 {
		t.Errorf("NextRunAt = %v, want a :30 UTC minute from the +05:30 zone", got.NextRunAt)
	}
}

func TestJobs_ValidationTable(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	tests := []struct {
		name string
		body string
	}{
		{"bad timezone", `{"command":["x"],"schedule":"* * * * *","timezone":"Mars/Base"}`},
		{"bad mode", `{"command":["x"],"schedule":"* * * * *","mode":"vm"}`},
		{"bad catch up", `{"command":["x"],"schedule":"* * * * *","catch_up":"always"}`},
		{"bad name", `{"command":["x"],"schedule":"* * * * *","name":"Has Space"}`},
		{"negative timeout", `{"command":["x"],"schedule":"* * * * *","timeout_seconds":-1}`},
		{"bad cron", `{"command":["x"],"schedule":"61 * * * *"}`},
		{"bad concurrency", `{"command":["x"],"schedule":"* * * * *","concurrency_policy":"maybe"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestJobs_DuplicateNameAndPerAppCap(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithJobLimits(2))
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	createJob(t, rt, cookie, `{"name":"a","command":["x"],"schedule":"* * * * *"}`)
	if rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs", `{"name":"a","command":["x"],"schedule":"* * * * *"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate name status = %d, want 409", rec.Code)
	}
	createJob(t, rt, cookie, `{"name":"b","command":["x"],"schedule":"* * * * *"}`)
	rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs", `{"name":"c","command":["x"],"schedule":"* * * * *"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "APP_JOBS_MAX_PER_APP") {
		t.Errorf("over-cap status = %d body = %s, want 409 naming the env var", rec.Code, rec.Body.String())
	}
}

func TestJobs_AddressByNameAndMergeUpdate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	created := createJob(t, rt, cookie, `{"name":"nightly","command":["x"],"schedule":"0 3 * * *","max_retries":3,"timeout_seconds":60}`)

	rec := doJobs(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/jobs/nightly", `{"schedule":"0 4 * * *"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.ID != created.ID || got.Schedule != "0 4 * * *" || got.MaxRetries != 3 || got.TimeoutSeconds != 60 || len(got.Command) != 1 {
		t.Errorf("updated = %+v, want schedule changed and every other field kept", got)
	}

	if rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs/nightly/disable", ""); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Errorf("disable = %d %s", rec.Code, rec.Body.String())
	}
}

func TestJobs_SpecManagedIsProtected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	now := time.Now().UTC()
	err := db.SaveScheduledTask(context.Background(), store.ScheduledTask{
		ID: "sct_spec", ServiceName: "web", Name: "from-spec", Command: []string{"x"}, Schedule: "* * * * *", Enabled: true,
		Source: store.ScheduledTaskSourceSpec, Mode: store.ScheduledTaskModeFresh, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if rec := doJobs(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/jobs/from-spec", `{"schedule":"0 1 * * *"}`); rec.Code != http.StatusConflict {
		t.Errorf("editing a spec job = %d, want 409", rec.Code)
	}
	if rec := doJobs(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/jobs/from-spec", ""); rec.Code != http.StatusConflict {
		t.Errorf("deleting a spec job = %d, want 409", rec.Code)
	}
	if rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs/from-spec/disable", ""); rec.Code != http.StatusOK {
		t.Errorf("disabling a spec job = %d, want 200", rec.Code)
	}
}

func TestJobs_RunNowReturnsRunIDAndHistoryEndpoints(t *testing.T) {
	starter := &fakeJobStarter{}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithScheduledTaskRunner(starter))
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	job := createJob(t, rt, cookie, `{"name":"nightly","command":["x"],"schedule":"0 3 * * *"}`)

	rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/jobs/nightly/run", "")
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"run_id":"jr_test"`) {
		t.Fatalf("run = %d %s", rec.Code, rec.Body.String())
	}
	if len(starter.tasks) != 1 || starter.tasks[0].ID != job.ID || starter.triggers[0] != store.ScheduledTaskTriggerManual {
		t.Errorf("starter calls = %v %v, want one manual run of the job", starter.tasks, starter.triggers)
	}

	finished := time.Now().UTC()
	exit := 1
	ctx := context.Background()
	rows := []store.ScheduledTaskRun{
		{ID: "jr_1", TaskID: job.ID, ServiceName: "web", Trigger: "schedule", Status: store.ScheduledTaskStatusRunning, StartedAt: finished.Add(-2 * time.Minute)},
		{ID: "jr_2", TaskID: job.ID, ServiceName: "web", Trigger: "manual", Status: store.ScheduledTaskStatusRunning, StartedAt: finished.Add(-time.Minute)},
	}
	for _, r := range rows {
		if err := db.CreateScheduledTaskRun(ctx, r); err != nil {
			t.Fatalf("seed run: %v", err)
		}
	}
	done := rows[1]
	done.Status, done.FinishedAt, done.ExitCode, done.Output, done.Attempts = store.ScheduledTaskStatusFailed, &finished, &exit, "boom", 2
	if err := db.FinishScheduledTaskRun(ctx, done); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	rec = doJobs(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/job-runs?status=failed", "")
	var runs []scheduledTaskRunResource
	_ = json.Unmarshal(rec.Body.Bytes(), &runs)
	if rec.Code != http.StatusOK || len(runs) != 1 || runs[0].ID != "jr_2" || runs[0].JobName != "nightly" || runs[0].Output != "" {
		t.Errorf("filtered history = %d %+v, want only jr_2 named nightly with no output in the list", rec.Code, runs)
	}

	rec = doJobs(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/jobs/nightly/runs/jr_2", "")
	var one scheduledTaskRunResource
	_ = json.Unmarshal(rec.Body.Bytes(), &one)
	if rec.Code != http.StatusOK || one.Output != "boom" || one.ExitCode == nil || *one.ExitCode != 1 || one.Attempts != 2 {
		t.Errorf("run detail = %d %+v", rec.Code, one)
	}
	if rec := doJobs(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/jobs/nightly/runs/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown run = %d, want 404", rec.Code)
	}
}

func TestJobs_LegacyRouteKeepsExecDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	rec := doJobs(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/scheduled-tasks", `{"command":["x"],"schedule":"* * * * *","enabled":true}`)
	var got scheduledTaskResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Mode != store.ScheduledTaskModeExec {
		t.Errorf("legacy create mode = %q, want exec", got.Mode)
	}
}
