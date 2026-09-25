package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePipelineRunner struct {
	started   []store.Pipeline
	cancelled []string
	nudges    int
}

func (f *fakePipelineRunner) Start(_ context.Context, p store.Pipeline, opt pipeline.StartOptions) (store.PipelineRun, error) {
	f.started = append(f.started, p)
	return store.PipelineRun{ID: "run1", PipelineID: p.ID, AppName: p.AppName, Number: 1, TriggerKind: opt.Trigger, Status: store.PipelineStatusQueued, CreatedAt: time.Now()}, nil
}

func (f *fakePipelineRunner) Cancel(_ context.Context, id string) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func (f *fakePipelineRunner) Rerun(context.Context, string, string) (store.PipelineRun, error) {
	return store.PipelineRun{ID: "run2", Number: 2, Status: store.PipelineStatusQueued, CreatedAt: time.Now()}, nil
}

func (f *fakePipelineRunner) DecideHold(context.Context, string, bool, string) (bool, error) {
	return false, nil
}

func (f *fakePipelineRunner) Nudge() { f.nudges++ }

const apiPipelineYAML = "version: 1\nname: ci\njobs:\n  t:\n    image: alpine\n    steps:\n      - run: echo hi\n"

func newPipelineRouter(t *testing.T) (*Router, *store.DB, *fakePipelineRunner) {
	t.Helper()
	db := openTestDB(t)
	runner := &fakePipelineRunner{}
	return NewRouter(discardLogger(), testBrand(), db, WithPipelines(db, runner)), db, runner
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPipelineRoutes_RequireAuth(t *testing.T) {
	rt, _, _ := newPipelineRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/pipelines"},
		{http.MethodPost, "/api/v1/apps/web/pipelines"},
		{http.MethodPut, "/api/v1/apps/web/pipelines/ci"},
		{http.MethodDelete, "/api/v1/apps/web/pipelines/ci"},
		{http.MethodPost, "/api/v1/apps/web/pipelines/ci/runs"},
		{http.MethodGet, "/api/v1/apps/web/pipeline-runs"},
		{http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1"},
		{http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/logs"},
		{http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/logs/stream"},
		{http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/cancel"},
		{http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/rerun"},
		{http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/approvals/1"},
		{http.MethodPost, "/api/v1/pipelines/validate"},
		{http.MethodGet, "/api/v1/pipelines/schema"},
	})
}

func TestPipelineRoutes_ReadOnlyTokenCannotMutate(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok", "plain-secret", []string{AbilityRead}, []providerRouteCase{
		{method: http.MethodPost, path: "/api/v1/apps/web/pipelines", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/apps/web/pipelines/ci/runs", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/apps/web/pipeline-runs/r1/cancel"},
		{method: http.MethodPost, path: "/api/v1/apps/web/pipeline-runs/r1/approvals/1", body: `{}`},
	})
}

func TestPipelineLifecycle(t *testing.T) {
	rt, db, runner := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	h := rt.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipelines", jsonBody(t, map[string]string{"yaml": "version: 1\njobs:\n  t:\n    steps:\n      - run: x\n"})))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"issues"`) || !strings.Contains(rec.Body.String(), "image is required") {
		t.Fatalf("invalid yaml: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipelines", jsonBody(t, map[string]string{"yaml": apiPipelineYAML})))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipelines", jsonBody(t, map[string]string{"yaml": apiPipelineYAML})))
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipelines", ""))
	var list []pipelineResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Name != "ci" || list[0].Jobs != 1 {
		t.Fatalf("list: %s (%v)", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipelines/ci/runs", `{"ref":"refs/heads/main"}`))
	if rec.Code != http.StatusAccepted || len(runner.started) != 1 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/pipelines/ci", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestPipelineRunDetailLogsCancelAndApproval(t *testing.T) {
	rt, db, runner := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	ctx := context.Background()
	now := time.Now().UTC()
	p, _ := db.SavePipeline(ctx, store.Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: apiPipelineYAML, Enabled: true, CreatedAt: now, UpdatedAt: now})
	_, _ = db.CreatePipelineRun(ctx, store.PipelineRun{ID: "r1", PipelineID: p.ID, AppName: "web", TriggerKind: "manual", Definition: apiPipelineYAML, CreatedAt: now})
	_ = db.CreatePipelineJobs(ctx, []store.PipelineJob{{ID: "j1", RunID: "r1", Key: "t", DisplayName: "t", NeedsJSON: "[]", MatrixJSON: "{}", OutputsJSON: "{}", Status: store.PipelineStatusWaitingApproval,
		Steps: []store.PipelineStep{{Index: 0, Name: "gate", Kind: "approval"}}}})
	_ = db.AppendPipelineLogs(ctx, []store.PipelineLogLine{{RunID: "r1", JobKey: "t", Stream: "stdout", Line: "hello", CreatedAt: now}})
	_ = db.CreatePipelineApproval(ctx, store.PipelineApproval{RunID: "r1", JobID: "j1", StepIndex: 0, RequiredAbility: AbilityDeploy, CreatedAt: now})
	h := rt.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1", ""))
	var run pipelineRunResource
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil || len(run.Jobs) != 1 || len(run.Jobs[0].Steps) != 1 || len(run.Approvals) != 1 || run.PipelineName != "ci" {
		t.Fatalf("detail: %s (%v)", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/other/pipeline-runs/r1", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-app run read: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/logs", ""))
	if !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("logs: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/approvals/"+jsonBody(t, run.Approvals[0].ID), `{"decision":"maybe"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decision: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/approvals/"+jsonBody(t, run.Approvals[0].ID), `{"decision":"approved"}`))
	if rec.Code != http.StatusOK || runner.nudges != 1 {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/approvals/"+jsonBody(t, run.Approvals[0].ID), `{"decision":"approved"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second decision: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/cancel", ""))
	if rec.Code != http.StatusAccepted || len(runner.cancelled) != 1 {
		t.Fatalf("cancel: %d", rec.Code)
	}
}

func TestPipelineCrossAppTargetsNeedRoot(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	seedScheduledTaskApp(t, db, "web")
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "t1", Name: "writer", TokenHash: hashToken("writer-secret"), Abilities: []string{AbilityWrite}}); err != nil {
		t.Fatal(err)
	}
	yaml := "version: 1\njobs:\n  p:\n    steps:\n      - uses: deploy\n        with: { service: other-app, image: x }\n"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/pipelines", strings.NewReader(jsonBody(t, map[string]string{"name": "x", "yaml": yaml})))
	req.Header.Set("Authorization", "Bearer writer-secret")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-app save by non-root: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPipelineValidateEndpoint(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/pipelines/validate", jsonBody(t, map[string]string{"yaml": "version: 1\njobs:\n  a:\n    needs: [b]\n    steps:\n      - uses: notify\n        with: { message: x }\n"})))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"valid":false`) || !strings.Contains(rec.Body.String(), "unknown job") {
		t.Fatalf("validate: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPipelineRoutesNotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipelines", ""))
	if rec.Code != http.StatusNotImplemented && rec.Code != http.StatusNotFound {
		t.Fatalf("unconfigured: %d", rec.Code)
	}
}
