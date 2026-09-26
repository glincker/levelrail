package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gateBuilder holds each build open until the test releases it, honors
// cancellation, and mimics the pipeline's commit point.
type gateBuilder struct {
	started      chan string
	release      chan struct{}
	stop         chan struct{}
	beforeCommit func(id string)
	afterCommit  chan struct{}

	mu        sync.Mutex
	committed []string
}

func newGateBuilder() *gateBuilder {
	return &gateBuilder{started: make(chan string, 16), release: make(chan struct{}), stop: make(chan struct{})}
}

func (g *gateBuilder) Deploy(ctx context.Context, req deploy.Request, _ func(build.ProgressEvent)) (string, error) {
	g.started <- req.AttemptID
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-g.release:
	case <-g.stop:
		return "", context.Canceled
	}
	if g.beforeCommit != nil {
		g.beforeCommit(req.AttemptID)
	}
	if req.Commit != nil {
		if err := req.Commit(); err != nil {
			return "", err
		}
	}
	g.mu.Lock()
	g.committed = append(g.committed, req.AttemptID)
	g.mu.Unlock()
	if g.afterCommit != nil {
		<-g.afterCommit
	}
	return req.ImageRepo + ":" + req.CommitSHA, nil
}

func (g *gateBuilder) DeploySpec(context.Context, deploy.MultiRequest, func(string, build.ProgressEvent)) ([]deploy.ServiceOutcome, error) {
	return nil, nil
}

func (g *gateBuilder) committedIDs() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.committed...)
}

func (g *gateBuilder) awaitStart(t *testing.T) string {
	t.Helper()
	select {
	case id := <-g.started:
		return id
	case <-time.After(3 * time.Second):
		t.Fatal("no build started within the deadline")
		return ""
	}
}

func (g *gateBuilder) assertNoStart(t *testing.T) {
	t.Helper()
	select {
	case id := <-g.started:
		t.Fatalf("build %s started, want none", id)
	case <-time.After(150 * time.Millisecond):
	}
}

func newQueueRouter(t *testing.T, opts ...Option) (*Router, *store.DB, *http.Cookie, *gateBuilder) {
	t.Helper()
	gb := newGateBuilder()
	t.Cleanup(func() { close(gb.stop) })
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, append([]Option{WithBuilder(gb), WithDeploySafety(db, nil)}, opts...)...)
	rt.fetch = newFakeFetch(t.TempDir(), nil).fetch
	seedWebApp(t, db)
	return rt, db, loginTestSession(t, rt, db), gb
}

func postBuild(t *testing.T, rt *Router, cookie *http.Cookie, app, ref string) triggerBuildResponse {
	t.Helper()
	body := `{"repo_url":"https://example.com/` + app + `.git","ref":"` + ref + `"}`
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/builds", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("build status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp triggerBuildResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func attemptStatus(t *testing.T, db *store.DB, id string) store.DeployAttempt {
	t.Helper()
	a, err := db.GetDeployAttempt(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return *a
}

func awaitAttemptStatus(t *testing.T, db *store.DB, id, want string) store.DeployAttempt {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		a := attemptStatus(t, db, id)
		if a.Status == want {
			return a
		}
		if time.Now().After(deadline) {
			t.Fatalf("attempt %s status = %q, want %q", id, a.Status, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func cancelDeploy(t *testing.T, rt *Router, cookie *http.Cookie, app, id string) (int, string) {
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/deploys/"+id+"/cancel", ""))
	return rec.Code, rec.Body.String()
}

func listAttempts(t *testing.T, rt *Router, cookie *http.Cookie, app string) []deployAttemptResource {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/"+app+"/deploy-attempts", ""))
	var out []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode attempts: %v (%s)", err, rec.Body.String())
	}
	return out
}

func TestQueue_SecondBuildWaitsThenRunsInOrder(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	first := postBuild(t, rt, cookie, "web", "main")
	if first.Status != "" || gb.awaitStart(t) != first.ID {
		t.Fatalf("first build should start at once: %+v", first)
	}
	second := postBuild(t, rt, cookie, "web", "feature")
	if second.Status != store.DeployAttemptStatusQueued || second.QueuePosition != 1 || second.WaitReason != "waiting for #"+first.ID {
		t.Fatalf("second = %+v", second)
	}
	third := postBuild(t, rt, cookie, "web", "other")
	if third.QueuePosition != 2 || third.WaitReason != "waiting for #"+second.ID {
		t.Fatalf("third = %+v", third)
	}

	var byID = map[string]deployAttemptResource{}
	for _, a := range listAttempts(t, rt, cookie, "web") {
		byID[a.ID] = a
	}
	q := byID[second.ID]
	if q.Status != "queued" || q.QueuePosition != 1 || q.BlockedBy != first.ID || q.QueuedAt == nil {
		t.Fatalf("listed queued attempt = %+v", q)
	}

	gb.release <- struct{}{}
	if got := gb.awaitStart(t); got != second.ID {
		t.Fatalf("next build = %s, want the queued attempt %s reused", got, second.ID)
	}
	awaitAttemptStatus(t, db, first.ID, store.DeployAttemptStatusSucceeded)
	gb.release <- struct{}{}
	if got := gb.awaitStart(t); got != third.ID {
		t.Fatalf("third build = %s, want %s", got, third.ID)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, third.ID, store.DeployAttemptStatusSucceeded)
	if got := gb.committedIDs(); len(got) != 3 || got[0] != first.ID || got[1] != second.ID || got[2] != third.ID {
		t.Fatalf("commit order = %v", got)
	}
}

func TestCancel_QueuedDeployNeverRuns(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	second := postBuild(t, rt, cookie, "web", "feature")

	code, body := cancelDeploy(t, rt, cookie, "web", second.ID)
	if code != http.StatusOK {
		t.Fatalf("cancel = %d %s", code, body)
	}
	a := attemptStatus(t, db, second.ID)
	if a.Status != store.DeployAttemptStatusCanceled || a.CanceledBy == "" || a.FinishedAt == nil {
		t.Fatalf("canceled attempt = %+v", a)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, first.ID, store.DeployAttemptStatusSucceeded)
	gb.assertNoStart(t)
	if code, _ := cancelDeploy(t, rt, cookie, "web", second.ID); code != http.StatusConflict {
		t.Fatalf("second cancel = %d, want 409", code)
	}
}

func TestCancel_RunningBuildStopsAndNextQueuedStarts(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	second := postBuild(t, rt, cookie, "web", "feature")

	code, body := cancelDeploy(t, rt, cookie, "web", first.ID)
	if code != http.StatusOK || !strings.Contains(body, `"status":"canceled"`) {
		t.Fatalf("cancel = %d %s", code, body)
	}
	if got := gb.awaitStart(t); got != second.ID {
		t.Fatalf("after cancel the queue should advance to %s, got %s", second.ID, got)
	}
	a := attemptStatus(t, db, first.ID)
	if a.Status != store.DeployAttemptStatusCanceled || a.Error != "" {
		t.Fatalf("canceled build must not be recorded as failed: %+v", a)
	}
	if ids := gb.committedIDs(); len(ids) != 0 {
		t.Fatalf("a canceled build wrote desired state: %v", ids)
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "levelrail/web:1" {
		t.Fatalf("serving release touched: %q", svc.Image)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, second.ID, store.DeployAttemptStatusSucceeded)
}

func TestCancel_BuildFinishedButCanceledBeforeCommitLeavesServingRelease(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	gb.beforeCommit = func(id string) { rt.cancels.Cancel(id, "alice") }
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	gb.release <- struct{}{}

	a := awaitAttemptStatus(t, db, first.ID, store.DeployAttemptStatusCanceled)
	if a.CanceledBy != "alice" {
		t.Fatalf("canceled_by = %q", a.CanceledBy)
	}
	if ids := gb.committedIDs(); len(ids) != 0 {
		t.Fatalf("desired state written after cancel: %v", ids)
	}
}

func TestCancel_AfterCommitIsConflict(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	gb.afterCommit = make(chan struct{})
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	gb.release <- struct{}{}
	deadline := time.Now().Add(3 * time.Second)
	for len(gb.committedIDs()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("build never committed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	code, body := cancelDeploy(t, rt, cookie, "web", first.ID)
	if code != http.StatusConflict || !strings.Contains(body, "cut over") {
		t.Fatalf("cancel after commit = %d %s", code, body)
	}
	close(gb.afterCommit)
	awaitAttemptStatus(t, db, first.ID, store.DeployAttemptStatusSucceeded)
}

func TestCancel_UnknownAndForeignAttempts(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "api", Image: "levelrail/api:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	if code, _ := cancelDeploy(t, rt, cookie, "web", "dep_missing"); code != http.StatusNotFound {
		t.Fatalf("missing = %d", code)
	}
	if code, _ := cancelDeploy(t, rt, cookie, "api", first.ID); code != http.StatusNotFound {
		t.Fatalf("other app's attempt = %d", code)
	}
	if code, _ := cancelDeploy(t, rt, cookie, "ghost", first.ID); code != http.StatusNotFound {
		t.Fatalf("missing app = %d", code)
	}
}

func TestSupersede_OnlyWhenEnabledAndOnlyQueuedOfSameBranch(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		rt, db, cookie, gb := newQueueRouter(t)
		if enabled {
			rec := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/cancel-superseded", `{"enabled":true}`))
			if rec.Code != http.StatusOK {
				t.Fatalf("put setting = %d %s", rec.Code, rec.Body.String())
			}
		}
		running := postBuild(t, rt, cookie, "web", "main")
		gb.awaitStart(t)
		old := postBuild(t, rt, cookie, "web", "feature")
		other := postBuild(t, rt, cookie, "web", "docs")
		newer := postBuild(t, rt, cookie, "web", "feature")

		gotOld := attemptStatus(t, db, old.ID)
		wantOld := store.DeployAttemptStatusQueued
		if enabled {
			wantOld = store.DeployAttemptStatusSuperseded
		}
		if gotOld.Status != wantOld {
			t.Fatalf("enabled=%v old status = %q, want %q", enabled, gotOld.Status, wantOld)
		}
		if enabled && gotOld.SupersededBy != newer.ID {
			t.Fatalf("superseded_by = %q, want %q", gotOld.SupersededBy, newer.ID)
		}
		if s := attemptStatus(t, db, other.ID).Status; s != store.DeployAttemptStatusQueued {
			t.Fatalf("other branch status = %q, want queued", s)
		}
		if s := attemptStatus(t, db, running.ID).Status; s != store.DeployAttemptStatusRunning {
			t.Fatalf("a build that already started must never be superseded, status = %q", s)
		}
		gb.release <- struct{}{}
		awaitAttemptStatus(t, db, running.ID, store.DeployAttemptStatusSucceeded)
	}
}

func TestCancelSupersededSetting_DefaultOffAndRoundTrip(t *testing.T) {
	rt, _, cookie, _ := newQueueRouter(t)
	get := func() string {
		return serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/cancel-superseded", "")).Body.String()
	}
	if !strings.Contains(get(), `"enabled":false`) {
		t.Fatalf("default = %s", get())
	}
	serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/cancel-superseded", `{"enabled":true}`))
	if !strings.Contains(get(), `"enabled":true`) {
		t.Fatalf("after put = %s", get())
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/cancel-superseded", `{"enabled":true}`)); rec.Code != http.StatusNotFound {
		t.Fatalf("missing app = %d", rec.Code)
	}
}

func TestQueue_GlobalConcurrencyLimitQueuesOtherApp(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t, WithDeployMaxConcurrent(1))
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "api", Image: "levelrail/api:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	other := postBuild(t, rt, cookie, "api", "main")
	if other.Status != store.DeployAttemptStatusQueued || other.WaitReason != "waiting for build capacity" {
		t.Fatalf("other app = %+v", other)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, first.ID, store.DeployAttemptStatusSucceeded)
	if got := gb.awaitStart(t); got != other.ID {
		t.Fatalf("capacity freed but %s started, want %s", got, other.ID)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, other.ID, store.DeployAttemptStatusSucceeded)
}

func TestQueue_RestartedControlPlaneResumesQueuedDeploy(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	svc, _ := db.GetDesiredService(context.Background(), "web")
	body, _ := json.Marshal(triggerBuildRequest{RepoURL: "https://example.com/web.git", Ref: "main"})
	now := time.Now()
	if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: "dep_q1", ServiceName: "web", Image: "web:main", Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusQueued, StartedAt: now, QueuedAt: &now,
		Snapshot: store.NewDeployAttemptSnapshot(*svc), HeldRequest: `{"kind":"build","build":` + string(body) + `}`,
	}); err != nil {
		t.Fatal(err)
	}
	_ = cookie
	rt.DrainDeployQueue(context.Background())
	if got := gb.awaitStart(t); got != "dep_q1" {
		t.Fatalf("started %s", got)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, "dep_q1", store.DeployAttemptStatusSucceeded)
}

func TestComputeWait(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q := func(id string, minutes int) store.DeployAttempt {
		ts := at.Add(time.Duration(minutes) * time.Minute)
		return store.DeployAttempt{ID: id, Status: store.DeployAttemptStatusQueued, QueuedAt: &ts, StartedAt: ts}
	}
	run := store.DeployAttempt{ID: "run", Status: store.DeployAttemptStatusRunning}
	tests := []struct {
		name        string
		app         []store.DeployAttempt
		target      string
		frozen, cap bool
		until       string
		wantPos     int
		wantReason  string
	}{
		{"behind running", []store.DeployAttempt{run, q("a", 1)}, "a", false, false, "", 1, "waiting for #run"},
		{"behind queued", []store.DeployAttempt{run, q("b", 2), q("a", 1)}, "b", false, false, "", 2, "waiting for #a"},
		{"freeze wins for head", []store.DeployAttempt{run, q("a", 1)}, "a", true, false, "2026-01-02T00:00:00Z", 1, "freeze window until 2026-01-02T00:00:00Z"},
		{"capacity", []store.DeployAttempt{q("a", 1)}, "a", false, true, "", 1, "waiting for build capacity"},
		{"free", []store.DeployAttempt{q("a", 1)}, "a", false, false, "", 1, "starting"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target store.DeployAttempt
			for _, a := range tt.app {
				if a.ID == tt.target {
					target = a
				}
			}
			w := computeWait(target, tt.app, tt.frozen, tt.until, tt.cap)
			if w.Position != tt.wantPos || w.Reason != tt.wantReason {
				t.Fatalf("wait = %+v", w)
			}
		})
	}
}

func TestHeldWaitReason(t *testing.T) {
	if got := heldWaitReason("Frozen until 2026-01-02T00:00:00Z"); got != "freeze window until 2026-01-02T00:00:00Z" {
		t.Fatalf("got %q", got)
	}
	if got := heldWaitReason("something else"); got != "something else" {
		t.Fatalf("got %q", got)
	}
}
