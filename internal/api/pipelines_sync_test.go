package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

type holdCall struct {
	run      string
	approved bool
}

type holdRunner struct {
	fakePipelineRunner
	calls   []holdCall
	pending bool
}

func (h *holdRunner) DecideHold(_ context.Context, runID string, approved bool, _ string) (bool, error) {
	h.calls = append(h.calls, holdCall{runID, approved})
	return h.pending, nil
}

type fakeSyncer struct {
	mu       sync.Mutex
	syncs    int
	pushRefs []string
	pushSHAs []string
	err      error
	order    *[]string
}

func (f *fakeSyncer) Sync(context.Context, string) (pipeline.SyncResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncs++
	return pipeline.SyncResult{SHA: "abc123", Dir: ".pipelines", Items: []pipeline.SyncItem{{File: "ci.yaml", Name: "ci", Outcome: pipeline.SyncCreated}}}, f.err
}

func (f *fakeSyncer) SyncOnPush(_ context.Context, _, ref, sha string) (pipeline.SyncResult, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushRefs = append(f.pushRefs, ref)
	f.pushSHAs = append(f.pushSHAs, sha)
	if f.order != nil {
		*f.order = append(*f.order, "sync")
	}
	return pipeline.SyncResult{}, true, f.err
}

type recordingEvents struct {
	mu     sync.Mutex
	events []pipeline.Event
	order  *[]string
	done   chan struct{}
}

func (r *recordingEvents) TriggerEvent(_ context.Context, _ string, ev pipeline.Event, _, _, _ string) []store.PipelineRun {
	r.mu.Lock()
	r.events = append(r.events, ev)
	if r.order != nil {
		*r.order = append(*r.order, "trigger")
	}
	r.mu.Unlock()
	if r.done != nil {
		r.done <- struct{}{}
	}
	return nil
}

func newSyncRouter(t *testing.T, syncer *fakeSyncer) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithPipelines(db, &holdRunner{}))
	rt.SetPipelineSync(syncer, db)
	return rt, db
}

func TestPipelineSyncRoutes_RequireAuth(t *testing.T) {
	rt, _ := newSyncRouter(t, &fakeSyncer{})
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/pipeline-sync"},
		{http.MethodPut, "/api/v1/apps/web/pipeline-sync"},
		{http.MethodPost, "/api/v1/apps/web/pipeline-sync"},
		{http.MethodGet, "/api/v1/apps/web/pipeline-triggers"},
		{http.MethodPost, "/api/v1/apps/web/pipeline-runs/r1/hold"},
	})
}

func TestPipelineSyncRoutes_ReadOnlyTokenCannotMutate(t *testing.T) {
	rt, db := newSyncRouter(t, &fakeSyncer{})
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok", "plain-secret", []string{AbilityRead}, []providerRouteCase{
		{method: http.MethodPut, path: "/api/v1/apps/web/pipeline-sync", body: `{"repo_is_truth":true}`},
		{method: http.MethodPost, path: "/api/v1/apps/web/pipeline-sync"},
		{method: http.MethodPost, path: "/api/v1/apps/web/pipeline-runs/r1/hold", body: `{"decision":"approved"}`},
	})
}

func TestPipelineSyncStatusSettingsAndRun(t *testing.T) {
	syncer := &fakeSyncer{}
	rt, db := newSyncRouter(t, syncer)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	h := rt.Handler()
	do := func(method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/apps/web/pipeline-sync", body))
		return rec
	}

	var st pipelineSyncStatus
	rec := do(http.MethodGet, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || rec.Code != http.StatusOK || st.Connected || st.RepoIsTruth {
		t.Fatalf("initial status: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodPut, `{"repo_is_truth":true}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || rec.Code != http.StatusOK || !st.RepoIsTruth {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	if saved, _ := db.GetPipelineSync(context.Background(), "web"); !saved.RepoIsTruth {
		t.Fatal("repo_is_truth was not persisted")
	}

	rec = do(http.MethodPost, "")
	if rec.Code != http.StatusOK || syncer.syncs != 1 || !strings.Contains(rec.Body.String(), `"outcome":"created"`) {
		t.Fatalf("sync now: %d %s", rec.Code, rec.Body.String())
	}

	syncer.err = pipeline.ErrNoRepo
	if rec = do(http.MethodPost, ""); rec.Code != http.StatusConflict {
		t.Fatalf("no repo: %d", rec.Code)
	}
	syncer.err = errors.New("network down")
	if rec = do(http.MethodPost, ""); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "network down") {
		t.Fatalf("fetch error: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPipelineSyncNotConfigured(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/pipeline-sync", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("unconfigured sync: %d", rec.Code)
	}
}

func TestPipelineRunHoldDecision(t *testing.T) {
	runner := &holdRunner{pending: true}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithPipelines(db, runner))
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	ctx := context.Background()
	now := time.Now().UTC()
	p, _ := db.SavePipeline(ctx, store.Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: apiPipelineYAML, Enabled: true, CreatedAt: now, UpdatedAt: now})
	_, _ = db.CreatePipelineRun(ctx, store.PipelineRun{ID: "r1", PipelineID: p.ID, AppName: "web", TriggerKind: "pull_request", Definition: apiPipelineYAML,
		HoldState: store.HoldPending, HoldReason: "fork", CreatedAt: now})
	h := rt.Handler()
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
		return rec
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1", ""))
	var run pipelineRunResource
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil || run.Hold == nil || run.Hold.State != store.HoldPending || run.Hold.Reason != "fork" {
		t.Fatalf("run hold view: %s (%v)", rec.Body.String(), err)
	}

	if rec = post("/api/v1/apps/web/pipeline-runs/r1/hold", `{"decision":"maybe"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decision: %d", rec.Code)
	}
	if rec = post("/api/v1/apps/other/pipeline-runs/r1/hold", `{"decision":"approved"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-app hold: %d", rec.Code)
	}
	if rec = post("/api/v1/apps/web/pipeline-runs/r1/hold", `{"decision":"approved"}`); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	runner.pending = false
	if rec = post("/api/v1/apps/web/pipeline-runs/r1/hold", `{"decision":"rejected"}`); rec.Code != http.StatusConflict {
		t.Fatalf("decided hold: %d", rec.Code)
	}
	if len(runner.calls) != 2 || !runner.calls[0].approved || runner.calls[1].approved {
		t.Fatalf("calls = %+v", runner.calls)
	}
}

func TestPipelineTriggerLogEndpoint(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	ctx := context.Background()
	_ = db.AddPipelineTriggerLog(ctx, store.PipelineTriggerLog{AppName: "web", Pipeline: "ci", Event: "pull_request", Decision: store.TriggerSkipped, Reason: "fork blocked", CreatedAt: time.Now().UTC()})
	_ = db.AddPipelineTriggerLog(ctx, store.PipelineTriggerLog{AppName: "other", Event: "push", Decision: store.TriggerStarted, Reason: "x", CreatedAt: time.Now().UTC()})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipeline-triggers", ""))
	var got []pipelineTriggerView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got) != 1 || got[0].Reason != "fork blocked" || got[0].Decision != "skipped" {
		t.Fatalf("triggers: %d %s (%v)", rec.Code, rec.Body.String(), err)
	}
}

func TestFirePipelinePullRequestPassesForkInfo(t *testing.T) {
	const payload = `{"action":"opened","number":1,"pull_request":{"head":{"ref":"feat","sha":"abc","repo":{"full_name":%q}},"base":{"ref":"main","repo":{"full_name":"acme/app"}}}}`
	tests := []struct {
		name     string
		headRepo string
		wantFork bool
	}{
		{"same repo", "acme/app", false},
		{"fork", "mallory/app", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newSyncRouter(t, &fakeSyncer{})
			events := &recordingEvents{}
			rt.SetPipelineEvents(events)
			body := strings.Replace(payload, "%q", `"`+tt.headRepo+`"`, 1)
			h := http.Header{}
			h.Set("X-GitHub-Event", "pull_request")
			pr, err := webhook.ParsePullRequestEventForProvider([]byte(body), h)
			if err != nil {
				t.Fatal(err)
			}
			rt.firePipelinePullRequest(context.Background(), "web", pr)
			if len(events.events) != 1 {
				t.Fatalf("events = %+v", events.events)
			}
			ev := events.events[0]
			if ev.Fork != tt.wantFork || ev.Branch != "main" || (tt.wantFork && ev.HeadRepo != "mallory/app") {
				t.Fatalf("event = %+v, want fork=%v", ev, tt.wantFork)
			}
		})
	}
}

func TestFirePipelinePushSyncsBeforeTriggering(t *testing.T) {
	var order []string
	syncer := &fakeSyncer{order: &order}
	rt, _ := newSyncRouter(t, syncer)
	events := &recordingEvents{order: &order, done: make(chan struct{}, 1)}
	rt.SetPipelineEvents(events)

	rt.firePipelinePushEvent(context.Background(), "web", webhook.PushEvent{Ref: "refs/heads/main", After: "abc"})
	select {
	case <-events.done:
	case <-time.After(5 * time.Second):
		t.Fatal("push never triggered pipelines")
	}
	if strings.Join(order, ",") != "sync,trigger" {
		t.Fatalf("order = %v, want sync before trigger", order)
	}
	if len(syncer.pushSHAs) != 1 || syncer.pushSHAs[0] != "abc" {
		t.Fatalf("sync must read the webhook commit, got %v", syncer.pushSHAs)
	}

	order = order[:0]
	rt.firePipelinePushEvent(context.Background(), "web", webhook.PushEvent{Ref: "refs/tags/v1", After: "abc"})
	<-events.done
	if strings.Join(order, ",") != "trigger" {
		t.Fatalf("a tag push must not sync: %v", order)
	}
}

func TestFirePipelinePushStillTriggersWhenSyncFails(t *testing.T) {
	syncer := &fakeSyncer{err: errors.New("clone failed")}
	rt, _ := newSyncRouter(t, syncer)
	events := &recordingEvents{done: make(chan struct{}, 1)}
	rt.SetPipelineEvents(events)
	rt.firePipelinePushEvent(context.Background(), "web", webhook.PushEvent{Ref: "refs/heads/main", After: "abc"})
	select {
	case <-events.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a failed sync must not block triggers")
	}
}

func TestFirePipelinePushTriggersWhenCommitUnavailable(t *testing.T) {
	syncer := &fakeSyncer{err: fmt.Errorf("fetch: %w", pipeline.ErrSHAUnavailable)}
	rt, _ := newSyncRouter(t, syncer)
	events := &recordingEvents{done: make(chan struct{}, 1)}
	rt.SetPipelineEvents(events)
	rt.firePipelinePushEvent(context.Background(), "web", webhook.PushEvent{Ref: "refs/heads/main", After: "abc"})
	select {
	case <-events.done:
	case <-time.After(5 * time.Second):
		t.Fatal("an unavailable commit must still trigger pipelines")
	}
}
