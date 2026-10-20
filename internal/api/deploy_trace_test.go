package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

const composeWithDeps = `
services:
  db:
    image: postgres:16
  web:
    image: nginx:1.27
    ports: ["8080:80"]
    depends_on:
      db:
        condition: service_healthy
`

func seedTracedAttempt(t *testing.T, db *store.DB, id string, steps ...store.DeployTraceStep) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: "levelrail/web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}
	for _, s := range steps {
		s.AttemptID = id
		if err := db.SaveDeployTraceStep(ctx, s); err != nil {
			t.Fatalf("seed step: %v", err)
		}
	}
}

func newDeployTraceRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(nil, testBrand(), db, WithDeployRecorder(deploylog.NewRecorder(nil, discardLogger()))), db
}

func TestDeployStepStream_FinishedImageDeployReplaysTrace(t *testing.T) {
	rt, db := newDeployTraceRouter(t)
	cookie := loginTestSession(t, rt, db)
	now := time.Now().UTC()
	seedTracedAttempt(t, db, "dep_img",
		store.DeployTraceStep{Step: "resolve_digest", Status: "done", Detail: "registry docker.io, digest sha256:abc, unchanged since the last deploy", StartedAt: now, EndedAt: now.Add(300 * time.Millisecond)},
		store.DeployTraceStep{Step: "image_pull", Status: "failed", Reason: "RegistryAuthDenied", Detail: "unauthorized: authentication required", StartedAt: now, EndedAt: now.Add(time.Second), DurationMS: 1000},
		store.DeployTraceStep{Step: "cutover", Status: "skipped", Reason: "NotCutOver", Detail: "previous container web-1 keeps serving", StartedAt: now, EndedAt: now},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_img/steps", "").WithContext(ctx)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	var events []sseStepEvent
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var ev sseStepEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				t.Fatalf("decode %q: %v", data, err)
			}
			events = append(events, ev)
		}
	}
	if len(events) != 3 {
		t.Fatalf("events = %+v, want the 3 stored steps and no synthesized build steps", events)
	}
	pull := events[1]
	if pull.Step != "image_pull" || pull.Status != "failed" || pull.Reason != "RegistryAuthDenied" || pull.DurationMS < 900 || pull.StartedAt == "" || pull.EndedAt == "" {
		t.Errorf("pull event = %+v, want failed/RegistryAuthDenied with timestamps and a duration", pull)
	}
	if events[2].Reason != "NotCutOver" || !strings.Contains(events[2].Detail, "keeps serving") {
		t.Errorf("cutover event = %+v, want the old-container-serving note", events[2])
	}
}

func TestDeployStepStream_FinishedWithoutTraceStillSynthesizes(t *testing.T) {
	rt, db := newDeployTraceRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTracedAttempt(t, db, "dep_old")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_old/steps", "").WithContext(ctx))
	if !strings.Contains(rec.Body.String(), `"step":"deploying"`) {
		t.Errorf("body = %q, want the legacy two-point synthesis for an attempt with no trace", rec.Body.String())
	}
}

func TestDeployStepStream_FollowsStepsAddedAfterFinish(t *testing.T) {
	t.Setenv(EnvTracePollInterval, "20ms")
	rt, db := newDeployTraceRouter(t)
	now := time.Now().UTC()
	seedTracedAttempt(t, db, "dep_late", store.DeployTraceStep{Step: "resolve_digest", Status: "done", StartedAt: now, EndedAt: now})

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)
	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/web/deploys/dep_late/steps", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	reader := bufio.NewReader(resp.Body)
	if first, err := nextSSEData(reader); err != nil || !strings.Contains(first, "resolve_digest") {
		t.Fatalf("first = %q, err = %v", first, err)
	}

	if err := db.SaveDeployTraceStep(context.Background(), store.DeployTraceStep{AttemptID: "dep_late", Step: "image_pull", Status: "running", Detail: "40%, 2/5 layers", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	next, err := nextSSEData(reader)
	if err != nil || !strings.Contains(next, `"step":"image_pull"`) || !strings.Contains(next, "40%") {
		t.Fatalf("late event = %q, err = %v, want the reconciler's pull progress", next, err)
	}
	if err := db.SaveDeployTraceStep(context.Background(), store.DeployTraceStep{AttemptID: "dep_late", Step: "image_pull", Status: "done", Detail: "100%", StartedAt: now, EndedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if done, err := nextSSEData(reader); err != nil || !strings.Contains(done, `"status":"done"`) {
		t.Fatalf("update event = %q, err = %v, want the same step finishing", done, err)
	}
}

func TestDeployLog_FinishedAttemptShowsTraceLines(t *testing.T) {
	db := openTestDB(t)
	fake := &fakeDeployLogQuerier{entries: map[string][]sseTestEntry{}}
	rt := NewRouter(nil, testBrand(), db, WithDeployLogQuerier(fake))
	cookie := loginTestSession(t, rt, db)
	now := time.Now().UTC()
	seedTracedAttempt(t, db, "dep_log",
		store.DeployTraceStep{Step: "image_pull", Status: "failed", Reason: "ImagePullFailed", Detail: "unexpected EOF", StartedAt: now, EndedAt: now.Add(time.Second), DurationMS: 1000},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_log/logs", "").WithContext(ctx))
	body := rec.Body.String()
	if !strings.Contains(body, "[trace] image_pull: failed [ImagePullFailed] unexpected EOF") || !strings.Contains(body, `"stream":"stderr"`) {
		t.Errorf("log stream = %q, want the failed pull as a stderr trace line", body)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_log/logs/download", ""))
	if !strings.Contains(rec.Body.String(), "[trace] image_pull: failed") {
		t.Errorf("download = %q, want the trace lines", rec.Body.String())
	}
}

func TestDeployCompose_TracesParsePlanAndPerServiceSteps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/shop/compose", composeWithDeps))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	attempts, err := db.ListDeployAttempts(ctx, "shop-web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %+v, err = %v, want one for shop-web", attempts, err)
	}
	steps, err := db.ListDeployTraceSteps(ctx, attempts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]store.DeployTraceStep{}
	for _, s := range steps {
		byKey[s.Key] = s
	}
	if p := byKey["compose_parse"]; p.Status != "done" || !strings.Contains(p.Detail, "2 service(s)") {
		t.Errorf("compose_parse = %+v", p)
	}
	plan := byKey["compose_plan"]
	if !strings.Contains(plan.Detail, "start order db -> web") || !strings.Contains(plan.Detail, "waits for db (healthy)") {
		t.Errorf("compose_plan = %q, want dependency order and the healthy condition", plan.Detail)
	}
	if svc := byKey["compose_service"]; !strings.Contains(svc.Detail, "nginx:1.27") || svc.Service != "shop-web" {
		t.Errorf("compose_service = %+v, want this service's own plan line", svc)
	}
	if d := byKey["resolve_digest"]; d.Status != "skipped" {
		t.Errorf("resolve_digest = %+v, want skipped (compose pulls the tag as given)", d)
	}
	if _, ok := byKey["config_validate"]; !ok {
		t.Error("config_validate step missing")
	}
}

func TestComposeStartOrder(t *testing.T) {
	svc := func(name string, deps ...string) store.DesiredService {
		return store.DesiredService{Name: "app-" + name, DependsOn: deps}
	}
	tests := []struct {
		name string
		in   []store.DesiredService
		want string
	}{
		{"alphabetical without deps", []store.DesiredService{svc("web"), svc("api"), svc("db")}, "api,db,web"},
		{"dependencies first", []store.DesiredService{svc("web", "api"), svc("api", "db"), svc("db")}, "db,api,web"},
		{"unknown dependency does not block", []store.DesiredService{svc("web", "ghost")}, "web"},
		{"cycle falls back to name order", []store.DesiredService{svc("a", "b"), svc("b", "a")}, "a,b"},
	}
	for _, tt := range tests {
		if got := strings.Join(composeStartOrder("app", tt.in), ","); got != tt.want {
			t.Errorf("%s: order = %s, want %s", tt.name, got, tt.want)
		}
	}
}
