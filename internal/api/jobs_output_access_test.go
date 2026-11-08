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

// jobOutputReadRoutes answer at the read tier but carry job output only for read_sensitive callers.
var jobOutputReadRoutes = []string{
	"GET /api/v1/jobs/runs/{id}",
	"GET /api/v1/apps/{name}/jobs/{id}",
	"GET /api/v1/apps/{name}/jobs",
	"GET /api/v1/apps/{name}/jobs/{id}/runs/{run}",
}

func seedJobWithOutput(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := t.Context()
	seedScheduledTaskApp(t, db, "web")
	now := time.Now().UTC()
	for _, task := range []store.ScheduledTask{
		{ID: "sct_os", ServiceName: "web", Command: []string{"env"}, Schedule: "@yearly", Name: "oneshot-jb_out",
			Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now},
		{ID: "sct_job", ServiceName: "web", Command: []string{"env"}, Schedule: "0 3 * * *", Name: "nightly", Enabled: true,
			Mode: store.ScheduledTaskModeFresh, Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.SaveScheduledTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	code := 0
	for _, id := range []string{"sct_os", "sct_job"} {
		run := store.ScheduledTaskRun{
			ID: "run_" + id, TaskID: id, ServiceName: "web", Trigger: store.ScheduledTaskTriggerAPI,
			Status: store.ScheduledTaskStatusSuccess, StartedAt: now, FinishedAt: &now, ExitCode: &code, Mode: store.ScheduledTaskModeFresh,
		}
		if err := db.CreateScheduledTaskRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		run.Output = "printed-value"
		if err := db.FinishScheduledTaskRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordScheduledTaskRun(ctx, "sct_job", now, store.ScheduledTaskStatusSuccess, "printed-value"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOneShotJob(ctx, store.OneShotJob{
		ID: "jb_out", App: "web", TaskID: "sct_os", State: "Succeeded", Completions: 1, Parallelism: 1, StartedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthzMatrix_JobOutputNeedsReadSensitive(t *testing.T) {
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	for _, key := range jobOutputReadRoutes {
		if !registered[key] {
			t.Errorf("%s is not registered in a routes*.go file", key)
		}
	}
	rt, db := newTestRouter(t)
	seedJobWithOutput(t, db)
	paths := []string{
		"/api/v1/jobs/runs/jb_out",
		"/api/v1/apps/web/jobs/sct_job",
		"/api/v1/apps/web/jobs",
		"/api/v1/apps/web/jobs/sct_job/runs/run_sct_job",
	}
	get := func(tok, path string) string {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		bearer(tok)(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s -> %d: %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	for _, p := range paths {
		body := get(reader, p)
		if strings.Contains(body, "printed-value") || !strings.Contains(body, "_withheld\":true") {
			t.Errorf("read tier GET %s must withhold output and say so: %s", p, body)
		}
	}
	var oneShot oneShotResponse
	if err := json.Unmarshal([]byte(get(reader, paths[0])), &oneShot); err != nil {
		t.Fatal(err)
	}
	if oneShot.ExitCode == nil || oneShot.RunStatus != store.ScheduledTaskStatusSuccess {
		t.Errorf("read tier must still see status and exit code: %+v", oneShot)
	}

	sensitive := seedMatrixToken(t, db, "sensitive", []string{AbilityRead, AbilityReadSensitive})
	for _, p := range paths {
		body := get(sensitive, p)
		if !strings.Contains(body, "printed-value") || strings.Contains(body, "_withheld") {
			t.Errorf("read_sensitive GET %s must carry the output: %s", p, body)
		}
	}
}
