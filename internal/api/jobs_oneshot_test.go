package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetOneShotJob_InterruptedByRestartIsFailedNotLost(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	if err := db.SaveOneShotJob(t.Context(), store.OneShotJob{
		ID: "jb_old", App: "web", State: "Running", Completions: 3, Parallelism: 1, Succeeded: 1, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/jobs/runs/jb_old", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got oneShotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != "Failed" || got.Succeeded != 1 || !strings.Contains(got.Message, "restarted") || got.FinishedAt == nil {
		t.Fatalf("job = %+v, want Failed with progress kept and a restart reason", got)
	}
}

func TestHandleGetOneShotJob_UnknownIs404(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/jobs/runs/jb_none", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleGetOneShotJob_CarriesLatestRunOutputTail(t *testing.T) {
	t.Setenv(envOneShotOutputTail, "5")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	ctx := t.Context()
	now := time.Now().UTC()
	if err := db.SaveScheduledTask(ctx, store.ScheduledTask{
		ID: "sct_os", ServiceName: "web", Command: []string{"echo"}, Schedule: "@yearly", Name: "oneshot-jb_out",
		Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	code := 3
	run := store.ScheduledTaskRun{
		ID: "run_os", TaskID: "sct_os", ServiceName: "web", Trigger: store.ScheduledTaskTriggerAPI,
		Status: store.ScheduledTaskStatusFailed, StartedAt: now, FinishedAt: &now, ExitCode: &code, Mode: store.ScheduledTaskModeFresh,
	}
	if err := db.CreateScheduledTaskRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run.Output = "hello world"
	if err := db.FinishScheduledTaskRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOneShotJob(ctx, store.OneShotJob{
		ID: "jb_out", App: "web", TaskID: "sct_os", State: "Failed", Completions: 1, Parallelism: 1, StartedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/jobs/runs/jb_out", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got oneShotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExitCode == nil || *got.ExitCode != 3 || got.Output != "world" || !got.OutputTruncated || got.RunStatus != "failed" {
		t.Fatalf("job = %+v, want exit 3 and the last 5 output bytes", got)
	}
}
