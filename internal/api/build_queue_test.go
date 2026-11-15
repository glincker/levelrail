package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func queueAttempt(t *testing.T, db *store.DB, id, app, source, status string) {
	t.Helper()
	now := time.Now().Add(-time.Hour)
	a := store.DeployAttempt{ID: id, ServiceName: app, Image: app + ":2", Source: source, Status: status, StartedAt: now}
	if status == store.DeployAttemptStatusQueued {
		a.QueuedAt = &now
	}
	if err := db.SaveDeployAttempt(context.Background(), a); err != nil {
		t.Fatalf("SaveDeployAttempt(%s) error = %v", id, err)
	}
}

func readQueue(t *testing.T, rt *Router, cookie *http.Cookie) buildQueueResource {
	t.Helper()
	var q buildQueueResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/builds/queue", "", &q); code != http.StatusOK {
		t.Fatalf("GET queue = %d", code)
	}
	return q
}

func TestBuildQueueTransitionsAuditAndRestart(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	if q := readQueue(t, rt, cookie); q.State != store.BuildQueueRunning || q.Running == nil || q.Queued == nil {
		t.Fatalf("fresh queue = %+v, want running with empty lists", q)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/queue/pause", `{"reason":"maintenance"}`, nil); code != http.StatusOK {
		t.Fatalf("pause = %d", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/queue/pause", `{}`, nil); code != http.StatusConflict {
		t.Fatalf("second pause = %d, want 409", code)
	}
	q := readQueue(t, rt, cookie)
	if q.State != store.BuildQueuePaused || q.Reason != "maintenance" || q.Actor == "" || q.Since == nil {
		t.Fatalf("paused queue = %+v", q)
	}

	// A new router over the same database is a restart: the pause survives it.
	again := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithDeploySafety(db, nil))
	if st, _ := again.buildQueueState(context.Background()); st.State != store.BuildQueuePaused || st.Reason != "maintenance" {
		t.Fatalf("after restart state = %+v, want paused", st)
	}

	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/queue/drain", ``, nil); code != http.StatusOK {
		t.Fatalf("drain = %d", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/queue/resume", ``, nil); code != http.StatusOK {
		t.Fatalf("resume = %d", code)
	}
	q = readQueue(t, rt, cookie)
	if q.State != store.BuildQueueRunning || len(q.Recent) != 3 {
		t.Fatalf("after resume = %+v, want running with 3 recorded changes", q)
	}
	if q.Recent[0].State != store.BuildQueueRunning || q.Recent[2].State != store.BuildQueuePaused || q.Recent[2].Actor == "" {
		t.Errorf("audit trail = %+v", q.Recent)
	}
}

func TestBuildQueuePausedHoldsQueuedBuildAndGate(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/queue/pause", `{"reason":"disk swap"}`, nil)
	queueAttempt(t, db, "att_q", "web", store.DeployAttemptSourceWebhook, store.DeployAttemptStatusQueued)

	rt.DrainDeployQueue(context.Background())
	got, err := db.GetDeployAttempt(context.Background(), "att_q")
	if err != nil || got.Status != store.DeployAttemptStatusQueued {
		t.Fatalf("paused queue started a build: %+v, %v", got, err)
	}
	q := readQueue(t, rt, cookie)
	if len(q.Queued) != 1 || !strings.Contains(q.Queued[0].WaitReason, "builds are paused by") || !strings.Contains(q.Queued[0].WaitReason, "disk swap") {
		t.Fatalf("queued = %+v, want the pause reason", q.Queued)
	}

	held := make(chan string, 4)
	done := make(chan error, 1)
	go func() { done <- rt.BuildQueueGate().Hold(context.Background(), func(m string) { held <- m }) }()
	if msg := <-held; !strings.HasPrefix(msg, "Waiting: builds are paused by") {
		t.Fatalf("gate message = %q", msg)
	}
	select {
	case err := <-done:
		t.Fatalf("gate returned while paused: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/queue/resume", ``, nil)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("gate after resume = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gate still holding after resume")
	}
}

func TestBuildQueueKillAllCancelsBuildsNotRollbacks(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	queueAttempt(t, db, "att_q", "web", store.DeployAttemptSourceManual, store.DeployAttemptStatusQueued)
	queueAttempt(t, db, "att_run", "web", store.DeployAttemptSourceWebhook, store.DeployAttemptStatusRunning)
	rt.cancels.Register("att_run")
	queueAttempt(t, db, "att_img", "api", store.DeployAttemptSourceImage, store.DeployAttemptStatusRunning)
	rt.cancels.Register("att_img")

	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/kill-all", `{"reason":"runaway"}`, nil); code != http.StatusForbidden {
		t.Fatalf("kill-all without step-up = %d, want 403", code)
	}
	body, _ := json.Marshal(map[string]any{"reason": "runaway", "pause": true, "step_up": map[string]string{"method": "password", "password": testAdminPassword}})
	var res killAllBuildsResponse
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/builds/kill-all", string(body), &res); code != http.StatusOK {
		t.Fatalf("kill-all = %d", code)
	}
	if len(res.Canceled) != 2 || len(res.Failed) != 0 || !res.Paused {
		t.Fatalf("kill-all result = %+v, want the queued and running build canceled", res)
	}
	for _, id := range []string{"att_q", "att_run"} {
		if got, _ := db.GetDeployAttempt(context.Background(), id); got.Status != store.DeployAttemptStatusCanceled {
			t.Errorf("%s status = %q, want canceled", id, got.Status)
		}
	}
	if got, _ := db.GetDeployAttempt(context.Background(), "att_img"); got.Status != store.DeployAttemptStatusRunning {
		t.Errorf("image rollout status = %q, kill-all must leave image deploys alone", got.Status)
	}
	if _, ok := rt.cancels.Canceled("att_img"); ok {
		t.Error("kill-all canceled an image rollout")
	}
	if !rt.buildsHeld(context.Background()) {
		t.Error("kill-all with pause left the queue running")
	}
}

func TestBuildQueueWritesNeedSensitiveAbility(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/api/v1/builds/queue/pause", "/api/v1/builds/queue/resume", "/api/v1/builds/queue/drain", "/api/v1/builds/kill-all"} {
		assertPlainWriteTokenForbidden(t, http.MethodPost, path, `{}`, "tok_write", "write-scoped-token", //nolint:gosec // fake fixture
			"holding or killing every build is write-sensitive")
	}
}
