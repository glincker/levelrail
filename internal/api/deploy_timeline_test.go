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
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploytimeline"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type timelineHarness struct {
	rt     *Router
	db     *store.DB
	tdb    *telemetry.DB
	cookie *http.Cookie
}

func newTimelineHarness(t *testing.T) timelineHarness {
	t.Helper()
	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithDeployRecorder(deploylog.NewRecorder(nil, discardLogger())),
		WithDeployGate(db, deploysafety.BuiltinDefaults()),
		WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)),
	)
	return timelineHarness{rt: rt, db: db, tdb: tdb, cookie: loginTestSession(t, rt, db)}
}

func (h timelineHarness) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(h.rt, authedRequest(t, h.cookie, http.MethodGet, target, ""))
}

func (h timelineHarness) seedDeploy(t *testing.T, id string, started time.Time, status string) {
	t.Helper()
	ctx := context.Background()
	if err := h.db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatal(err)
	}
	if err := h.db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: "web", Image: "img:" + id, Source: store.DeployAttemptSourceGitPush, Branch: "main",
		CommitSHA: "abcdef1234567", CommitMessage: "ship it", Author: "ana", Status: store.DeployAttemptStatusRunning, StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.FinishDeployAttempt(ctx, id, status, started.Add(40*time.Second), ""); err != nil {
		t.Fatal(err)
	}
}

func (h timelineHarness) seedStory(t *testing.T, id string, started time.Time) {
	t.Helper()
	ctx := context.Background()
	h.seedDeploy(t, id, started, store.DeployAttemptStatusSucceeded)
	for _, s := range []store.DeployTraceStep{
		{Step: "building", Status: "done", StartedAt: started.Add(time.Second), EndedAt: started.Add(20 * time.Second)},
		{Step: "pushing", Status: "done", StartedAt: started.Add(20 * time.Second), EndedAt: started.Add(25 * time.Second)},
		{Step: "readiness_probe", Status: "done", StartedAt: started.Add(26 * time.Second), EndedAt: started.Add(28 * time.Second)},
		{Step: "deploy_summary", Status: "done", StartedAt: started.Add(39 * time.Second), EndedAt: started.Add(40 * time.Second)},
	} {
		s.AttemptID = id
		if err := h.db.SaveDeployTraceStep(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	cut := started.Add(30 * time.Second)
	for _, e := range []store.DeployGateEvent{
		{Phase: deploysafety.PhaseCutover, Status: deploysafety.StatusPassed, At: cut, Message: "traffic moved"},
		{Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusFailed, Reason: deploysafety.ReasonWatchCrashloop, At: cut.Add(10 * time.Second), Message: "3 restarts"},
	} {
		e.DeployAttemptID, e.ServiceName = id, "web"
		if err := h.db.RecordGateEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	h.seedImpactMetrics(t, cut)
}

func (h timelineHarness) seedImpactMetrics(t *testing.T, cut time.Time) {
	t.Helper()
	write := func(at time.Time, total, errs float64) {
		err := h.tdb.WriteSamples(context.Background(), []telemetry.Sample{
			{ResourceID: "service:web", Metric: telemetry.MetricHTTPRequests, Timestamp: at, Value: total},
			{ResourceID: "service:web", Metric: telemetry.MetricHTTPResponses5xx, Timestamp: at, Value: errs},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	write(cut.Add(-time.Minute), 600, 6)
	write(cut.Add(5*time.Second), 300, 60)
}

func setTimelineEnv(t *testing.T) {
	t.Helper()
	t.Setenv(deploytimeline.EnvImpactWindow, "2m")
	t.Setenv(deploytimeline.EnvSettleIdle, "1s")
	t.Setenv(EnvTracePollInterval, "20ms")
}

func decodeTimeline(t *testing.T, rec *httptest.ResponseRecorder) deploytimeline.Timeline {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var tl deploytimeline.Timeline
	if err := json.Unmarshal(rec.Body.Bytes(), &tl); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return tl
}

func eventCodes(tl deploytimeline.Timeline) []string {
	out := make([]string, 0, len(tl.Events))
	for _, e := range tl.Events {
		out = append(out, e.Code)
	}
	return out
}

func indexOf(codes []string, want string) int {
	for i, c := range codes {
		if c == want {
			return i
		}
	}
	return -1
}

func TestDeployTimeline_AssemblesOrderedStory(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	started := time.Now().UTC().Add(-10 * time.Minute)
	h.seedStory(t, "dep_a", started)
	if err := h.db.UpsertHookRun(context.Background(), store.HookRun{ServiceName: "web", HookType: "pre_deploy", Command: "migrate", Success: true, RanAt: started.Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}

	tl := decodeTimeline(t, h.get(t, "/api/v1/deployments/dep_a/timeline"))
	codes := eventCodes(tl)
	order := []string{"trigger.git_push", "build.building", "hook.pre_deploy", "push", "step.readiness_probe", "cutover.passed", "watch.crashloop", "traffic.impact", "outcome.succeeded"}
	last := -1
	for _, c := range order {
		i := indexOf(codes, c)
		if i < 0 {
			t.Fatalf("missing %s in %v", c, codes)
		}
		if c != "traffic.impact" && c != "outcome.succeeded" && i < last {
			t.Fatalf("%s out of order in %v", c, codes)
		}
		last = i
	}
	if tl.Trigger.Author != "ana" || tl.Trigger.Branch != "main" {
		t.Fatalf("trigger = %+v", tl.Trigger)
	}
	if tl.Impact == nil || !tl.Impact.Before.HasTraffic || !tl.Impact.After.HasTraffic {
		t.Fatalf("impact = %+v", tl.Impact)
	}
	if tl.Impact.After.ErrorRate5xx <= tl.Impact.Before.ErrorRate5xx {
		t.Fatalf("expected 5xx to rise after cutover: %+v", tl.Impact)
	}
	if len(tl.Phases) == 0 {
		t.Fatal("phases missing")
	}
}

func TestDeployTimeline_FreezesAndReplays(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	h.seedStory(t, "dep_f", time.Now().UTC().Add(-time.Hour))

	first := decodeTimeline(t, h.get(t, "/api/v1/deployments/dep_f/timeline"))
	if !first.Frozen {
		t.Fatal("settled deploy should be frozen on first read")
	}
	if _, err := h.db.GetDeployTimeline(context.Background(), "dep_f"); err != nil {
		t.Fatalf("frozen row missing: %v", err)
	}
	if err := h.db.SaveDeployTraceStep(context.Background(), store.DeployTraceStep{AttemptID: "dep_f", Step: "container_start", Status: "done", StartedAt: time.Now().Add(-59 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	second := decodeTimeline(t, h.get(t, "/api/v1/deployments/dep_f/timeline"))
	if !second.Frozen || second.FrozenAt == nil || len(second.Events) != len(first.Events) {
		t.Fatalf("replay changed: frozen=%v first=%d second=%d", second.Frozen, len(first.Events), len(second.Events))
	}
}

func TestDeployTimeline_RetentionExpiresReplay(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	h.seedStory(t, "dep_old", time.Now().UTC().Add(-time.Hour))
	if err := h.db.SaveDeployTimeline(context.Background(), store.DeployTimelineRow{
		AttemptID: "dep_old", ServiceName: "web", FrozenAt: time.Now().Add(-100 * 24 * time.Hour), Payload: []byte(`{"deploy_id":"dep_old","events":[]}`),
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(deploytimeline.EnvRetention, "24h")
	tl := decodeTimeline(t, h.get(t, "/api/v1/deployments/dep_old/timeline"))
	if len(tl.Events) == 0 {
		t.Fatal("an expired replay must fall back to live assembly")
	}
}

func TestDeployTimeline_PruneRemovesOldRows(t *testing.T) {
	h := newTimelineHarness(t)
	ctx := context.Background()
	for id, age := range map[string]time.Duration{"keep": time.Hour, "drop": 40 * 24 * time.Hour} {
		if err := h.db.SaveDeployTimeline(ctx, store.DeployTimelineRow{AttemptID: id, ServiceName: "web", FrozenAt: time.Now().Add(-age), Payload: []byte("{}")}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := h.db.PruneDeployTimelines(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
	if _, err := h.db.GetDeployTimeline(ctx, "keep"); err != nil {
		t.Fatalf("keep missing: %v", err)
	}
	if _, err := h.db.GetDeployTimeline(ctx, "drop"); err == nil {
		t.Fatal("drop should be gone")
	}
}

func TestDeployTimeline_NotFoundAndVisibility(t *testing.T) {
	h := newTimelineHarness(t)
	if rec := h.get(t, "/api/v1/deployments/nope/timeline"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec := h.get(t, "/api/v1/deployments/nope/timeline/stream"); rec.Code != http.StatusNotFound {
		t.Fatalf("stream status = %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/nope/timeline", nil)
	if rec := serve(h.rt, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", rec.Code)
	}
}

func TestDeployTimeline_Compare(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	h.seedStory(t, "dep_1", time.Now().UTC().Add(-2*time.Hour))
	h.seedDeploy(t, "dep_2", time.Now().UTC().Add(-time.Hour), store.DeployAttemptStatusSucceeded)

	rec := h.get(t, "/api/v1/deployments/dep_1/timeline/compare?with=dep_2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var c deploytimeline.Comparison
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.A.DeployID != "dep_1" || c.B.DeployID != "dep_2" || indexOf(c.OnlyA, "watch.crashloop") < 0 {
		t.Fatalf("comparison = a:%s b:%s onlyA:%v", c.A.DeployID, c.B.DeployID, c.OnlyA)
	}
	if rec := h.get(t, "/api/v1/deployments/dep_1/timeline/compare"); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing with = %d", rec.Code)
	}
	if rec := h.get(t, "/api/v1/deployments/dep_1/timeline/compare?with=zzz"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown with = %d", rec.Code)
	}
}

func readTimelineMessages(t *testing.T, body string) []timelineMessage {
	t.Helper()
	var out []timelineMessage
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var m timelineMessage
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("decode %q: %v", data, err)
		}
		out = append(out, m)
	}
	return out
}

func TestDeployTimeline_StreamSettledDeployEndsWithSnapshot(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	h.seedStory(t, "dep_s", time.Now().UTC().Add(-time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := authedRequest(t, h.cookie, http.MethodGet, "/api/v1/deployments/dep_s/timeline/stream", "").WithContext(ctx)
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)

	msgs := readTimelineMessages(t, rec.Body.String())
	if len(msgs) < 3 || msgs[0].Type != timelineMsgSnapshot || msgs[len(msgs)-1].Type != timelineMsgEnd {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs[0].Timeline == nil || indexOf(eventCodes(*msgs[0].Timeline), "watch.crashloop") < 0 {
		t.Fatalf("snapshot lacks the watch signal: %+v", msgs[0].Timeline)
	}
	if _, err := h.db.GetDeployTimeline(context.Background(), "dep_s"); err != nil {
		t.Fatalf("stream end should freeze the timeline: %v", err)
	}
}

func TestDeployTimeline_StreamFollowsLiveDeploy(t *testing.T) {
	setTimelineEnv(t)
	h := newTimelineHarness(t)
	ctx := context.Background()
	started := time.Now().UTC().Add(-5 * time.Second)
	if err := h.db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_live", ServiceName: "web", Image: "img:live", Source: store.DeployAttemptSourceManual, Status: store.DeployAttemptStatusRunning, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SaveDeployTraceStep(ctx, store.DeployTraceStep{AttemptID: "dep_live", Step: "building", Status: "running", StartedAt: started.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req := authedRequest(t, h.cookie, http.MethodGet, "/api/v1/deployments/dep_live/timeline/stream", "").WithContext(reqCtx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.rt.Handler().ServeHTTP(rec, req)
	}()

	time.Sleep(150 * time.Millisecond)
	if err := h.db.SaveDeployTraceStep(ctx, store.DeployTraceStep{AttemptID: "dep_live", Step: "building", Status: "done", StartedAt: started.Add(time.Second), EndedAt: started.Add(3 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SaveDeployTraceStep(ctx, store.DeployTraceStep{AttemptID: "dep_live", Step: "deploy_summary", Status: "done", StartedAt: started.Add(4 * time.Second), EndedAt: started.Add(4 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.FinishDeployAttempt(ctx, "dep_live", store.DeployAttemptStatusSucceeded, started.Add(4*time.Second), ""); err != nil {
		t.Fatal(err)
	}
	<-done

	msgs := readTimelineMessages(t, rec.Body.String())
	var snapshotLive, sawBuildDone, sawEnd bool
	for _, m := range msgs {
		switch {
		case m.Type == timelineMsgSnapshot:
			snapshotLive = m.Live && m.Verdict == deploytimeline.VerdictRunning
		case m.Type == timelineMsgEvent && m.Event != nil && m.Event.Code == "build.building" && m.Event.Status == deploytimeline.StatusOK:
			sawBuildDone = true
		case m.Type == timelineMsgEnd:
			sawEnd = true
		}
	}
	if !snapshotLive || !sawBuildDone || !sawEnd {
		t.Fatalf("live=%v buildDone=%v end=%v msgs=%d", snapshotLive, sawBuildDone, sawEnd, len(msgs))
	}
}

func TestTimelineActorFromAudit(t *testing.T) {
	h := newTimelineHarness(t)
	started := time.Now().UTC().Add(-time.Minute)
	err := h.db.SaveAuditEntry(context.Background(), store.AuditEntry{
		ID: "a1", ActorType: "token", ActorID: "tok1", ActorName: "ci-bot", Method: http.MethodPost, Path: "/api/v1/apps/web/deploys",
		StatusCode: http.StatusAccepted, CreatedAt: store.FormatAuditTime(started.Add(-2 * time.Second)), ClientKind: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	actor, kind := h.rt.timelineActor(context.Background(), store.DeployAttempt{ID: "x", ServiceName: "web", Source: store.DeployAttemptSourceManual, StartedAt: started}, time.Minute)
	if actor != "ci-bot" || kind != "cli" {
		t.Fatalf("actor = %q kind = %q", actor, kind)
	}
	actor, _ = h.rt.timelineActor(context.Background(), store.DeployAttempt{ID: "x", ServiceName: "web", Source: store.DeployAttemptSourceGitPush, StartedAt: started}, time.Minute)
	if actor != "" {
		t.Fatalf("git push should not borrow an audit actor, got %q", actor)
	}
}
