package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// gateServer answers the readiness probe 200 only once opened.
type gateServer struct {
	srv  *httptest.Server
	open atomic.Bool
}

func newGateServer(t *testing.T, open bool) *gateServer {
	t.Helper()
	g := &gateServer{}
	g.open.Store(open)
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if g.open.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

type policyStore struct {
	historyStore
	policy store.RolloutPolicy
}

func (p *policyStore) GetRolloutPolicy(context.Context, string) (store.RolloutPolicy, bool, error) {
	return p.policy, true, nil
}

func takeoverDesired(image string, budget time.Duration) *store.DesiredService {
	return &store.DesiredService{
		Name: "web", Image: image, Port: 80, Replicas: 1, Strategy: strategyBlueGreen,
		Volumes: []store.ServiceVolume{{Name: "uploads", ContainerPath: "/data"}},
		Health: &store.ServiceHealth{
			Readiness:    &store.ServiceProbe{Path: "/healthz", Interval: 5 * time.Millisecond, Timeout: 50 * time.Millisecond},
			ReadyTimeout: budget,
		},
	}
}

// takeoverRig seeds a serving v1 release that holds the server port.
type takeoverRig struct {
	rt   *fakeRuntime
	st   *historyStore
	v1   string
	v2   string
	port int
}

func newTakeoverRig(t *testing.T, g *gateServer, desired *store.DesiredService, streamed bool) *takeoverRig {
	t.Helper()
	port := serverPort(t, g.srv)
	r := &takeoverRig{rt: newFakeRuntime(port), port: port}
	r.v1 = replicaContainerName("web", "img:v1", "", 0)
	r.v2 = replicaContainerName("web", desired.Image, "", 0)
	r.rt.seed(r.v1, true)
	r.rt.containers[r.v1].Image = "img:v1"
	r.rt.containers[r.v1].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: port}}
	r.st = &historyStore{
		fakeStore: fakeStore{svc: desired},
		attempts:  []store.DeployAttempt{{ID: "d1", Image: "img:v1", Status: store.DeployAttemptStatusSucceeded}},
	}
	if streamed {
		r.st.streams = []store.AppStream{{ID: "s1", ServiceName: "web", ContainerPort: 80, HostPort: port, Protocol: "tcp"}}
	}
	return r
}

func (r *takeoverRig) overlaps() int {
	r.rt.mu.Lock()
	defer r.rt.mu.Unlock()
	return r.rt.overlapStarts
}

func (r *takeoverRig) controller(opts ...Option) *Controller {
	base := []Option{WithReviveWatch(10 * time.Millisecond), WithRestartBackoff(NewRestartBackoff(time.Millisecond, time.Millisecond, time.Millisecond))}
	return New("web", r.st, r.rt, append(base, opts...)...)
}

// The revive guard: while a takeover candidate runs, the stopped old
// release must not be started beside it.
func TestTakeover_ReviveNeverStartsOldBesideCandidate(t *testing.T) {
	g := newGateServer(t, false)
	r := newTakeoverRig(t, g, takeoverDesired("img:v2", 5*time.Second), true)
	c := r.controller()

	done := make(chan error, 1)
	var res reconcile.Result
	go func() {
		var err error
		res, err = c.Reconcile(context.Background())
		done <- err
	}()
	time.Sleep(150 * time.Millisecond)
	if isRunning(r.rt, r.v1) {
		t.Fatal("the old release was started while the candidate was still waiting for readiness")
	}
	g.open.Store(true)
	if err := <-done; err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want Ready", cond)
	}
	if n := r.overlaps(); n != 0 {
		t.Fatalf("overlapping starts = %d, want 0", n)
	}
	if isRunning(r.rt, r.v1) || !isRunning(r.rt, r.v2) {
		t.Fatalf("running = %v, want only the candidate", runningNames(r.rt))
	}
}

func TestTakeover_DeferredPendingDoesNotReviveOld(t *testing.T) {
	g := newGateServer(t, false)
	r := newTakeoverRig(t, g, takeoverDesired("img:v2", time.Minute), true)
	tr := NewPendingReadiness()
	run := func() (reconcile.Result, error) {
		return r.controller(WithReadinessRequeue(tr, requeueDelay)).Reconcile(context.Background())
	}
	for i := 0; i < 5; i++ {
		res, err := run()
		assertPending(t, res, err)
		if isRunning(r.rt, r.v1) {
			t.Fatalf("pass %d started the old release beside the pending candidate", i)
		}
		time.Sleep(requeueDelay)
	}
	g.open.Store(true)
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := run()
		if len(res.Conditions) > 0 && res.Conditions[0].Reason != reasonReadinessPending {
			if err != nil {
				t.Fatalf("settled with error %v", err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("candidate never settled")
		}
		time.Sleep(requeueDelay)
	}
	if n := r.overlaps(); n != 0 {
		t.Fatalf("overlapping starts = %d, want 0", n)
	}
}

func TestTakeover_FailedCandidateRestoresOldOnceAndBacksOff(t *testing.T) {
	g := newGateServer(t, false)
	r := newTakeoverRig(t, g, takeoverDesired("img:v2", 150*time.Millisecond), true)
	c := r.controller(WithPinnedPortRetry(time.Hour))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the readiness failure")
	}
	if !isRunning(r.rt, r.v1) || isRunning(r.rt, r.v2) {
		t.Fatalf("running = %v, want the old release restored after the verdict", runningNames(r.rt))
	}
	if n := r.overlaps(); n != 0 {
		t.Fatalf("overlapping starts = %d, want the old release started only after the candidate was gone", n)
	}
	stops := r.rt.stopCalls
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("second Reconcile() error = nil, want the backoff error")
	}
	if cond := conditionOf(t, res); cond.Reason != reasonPinnedPortWait {
		t.Fatalf("reason = %q, want %s", cond.Reason, reasonPinnedPortWait)
	}
	if r.rt.stopCalls != stops || !isRunning(r.rt, r.v1) {
		t.Fatalf("stop calls %d -> %d, running %v: the serving release was stopped again inside the backoff", stops, r.rt.stopCalls, runningNames(r.rt))
	}
}

// The narrowing: none of these apps may lose the blue-green overlap.
func TestTakeover_NarrowAppsKeepOverlap(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(d *store.DesiredService)
		streamed bool
	}{
		{"read-only bind and docker socket", func(d *store.DesiredService) {
			d.Volumes = nil
			d.BindMounts = []store.ServiceBindMount{
				{HostPath: "/etc/app", ContainerPath: "/etc/app", ReadOnly: true},
				{HostPath: "/var/run/docker.sock", ContainerPath: "/var/run/docker.sock"},
				{HostPath: "/srv/other.sock", ContainerPath: "/run/other.sock"},
			}
		}, true},
		{"two replicas with a volume", func(d *store.DesiredService) { d.Replicas = 2 }, true},
		{"writable volume without ports or streams", func(*store.DesiredService) {}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateServer(t, true)
			d := takeoverDesired("img:v2", 5*time.Second)
			tc.mutate(d)
			r := newTakeoverRig(t, g, d, tc.streamed)
			if tc.streamed {
				// A stream on a port the old release does not hold: only the data rule could stop it.
				r.st.streams[0].HostPort = r.port + 1
			}
			if _, err := r.controller().Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if r.overlaps() == 0 {
				t.Fatal("the old release was stopped before the new one started, want the overlap kept")
			}
		})
	}
}

func TestTakeover_WritableBindWithStreamStopsFirst(t *testing.T) {
	g := newGateServer(t, true)
	d := takeoverDesired("img:v2", 5*time.Second)
	d.Volumes = nil
	d.BindMounts = []store.ServiceBindMount{{HostPath: t.TempDir(), ContainerPath: "/data"}}
	r := newTakeoverRig(t, g, d, true)
	if _, err := r.controller().Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if n := r.overlaps(); n != 0 {
		t.Fatalf("overlapping starts = %d, want the writable bind released first", n)
	}
}

func TestTakeover_MissingBindSourceKeepsServing(t *testing.T) {
	g := newGateServer(t, true)
	d := takeoverDesired("img:v2", 5*time.Second)
	d.Volumes = nil
	d.BindMounts = []store.ServiceBindMount{{HostPath: filepath.Join(t.TempDir(), "missing"), ContainerPath: "/data"}}
	r := newTakeoverRig(t, g, d, true)
	if _, err := r.controller().Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the missing bind source refused")
	}
	if r.rt.stopCalls != 0 || !isRunning(r.rt, r.v1) {
		t.Fatalf("stop calls = %d, running %v: the serving release was stopped for a bind source that is not there", r.rt.stopCalls, runningNames(r.rt))
	}
}

func TestTakeover_StopFailureOnSecondReleaseRestoresFirst(t *testing.T) {
	g := newGateServer(t, true)
	r := newTakeoverRig(t, g, takeoverDesired("img:v3", 5*time.Second), true)
	extra := replicaContainerName("web", "img:v1", "n1", 0)
	r.rt.seed(extra, true)
	r.rt.containers[extra].Image = "img:v1"
	r.rt.containers[extra].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: r.port}}
	r.rt.stopErrOnCall = 2
	if _, err := r.controller().Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the stop failure")
	}
	// The first stopped release was started again; later cleanup of superseded
	// previous releases is a separate, older behavior.
	if r.rt.startCalls != 1 {
		t.Fatalf("start calls = %d, want the first stopped release restored once", r.rt.startCalls)
	}
}

func TestTakeover_RolloutBudgetRefusesStopFirst(t *testing.T) {
	for _, pol := range []store.RolloutPolicy{{MaxUnavailable: "0"}, {MinAvailable: "1"}} {
		g := newGateServer(t, true)
		r := newTakeoverRig(t, g, takeoverDesired("img:v2", 5*time.Second), true)
		ps := &policyStore{historyStore: *r.st, policy: pol}
		c := New("web", ps, r.rt)
		_, err := c.Reconcile(context.Background())
		if err == nil || !strings.Contains(err.Error(), "rollout budget") {
			t.Fatalf("policy %+v: error = %v, want the rollout budget refusal", pol, err)
		}
		if r.rt.stopCalls != 0 || r.rt.createCalls != 0 || !isRunning(r.rt, r.v1) {
			t.Fatalf("policy %+v: stops=%d creates=%d, want the serving release untouched", pol, r.rt.stopCalls, r.rt.createCalls)
		}
	}
}

func TestTakeover_CandidateRecordsReleasedAndRestartRestoresIt(t *testing.T) {
	g := newGateServer(t, false)
	r := newTakeoverRig(t, g, takeoverDesired("img:v2", time.Minute), true)
	first := NewPendingReadiness()
	res, err := r.controller(WithReadinessRequeue(first, requeueDelay)).Reconcile(context.Background())
	assertPending(t, res, err)
	if got := r.rt.containers[r.v2].Labels[releasedLabelKey]; got != r.v1 {
		t.Fatalf("released label = %q, want %q", got, r.v1)
	}

	// A control plane restart: a fresh tracker knows nothing of the stopped release.
	r.rt.setExitState(r.v2, &docker.ExitState{Running: true, StartedAt: time.Now().Add(-time.Second)})
	restarted := NewPendingReadiness()
	res, err = r.controller(WithReadinessRequeue(restarted, requeueDelay)).Reconcile(context.Background())
	assertPending(t, res, err)
	r.rt.crashContainer(r.v2, false, 1)
	if _, err = r.controller(WithReadinessRequeue(restarted, requeueDelay)).Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the candidate failure")
	}
	if !isRunning(r.rt, r.v1) {
		t.Fatalf("running = %v, want the previous release restored after the restart", runningNames(r.rt))
	}
}

func TestTakeover_HandoffBackoffMapIsRaceFree(_ *testing.T) {
	c := New("web", &fakeStore{svc: takeoverDesired("img:v2", time.Second)}, newFakeRuntime(0))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.noteHandoffFailed("a")
				c.handoffBlockedUntil("a")
				c.clearHandoffFailed("a")
			}
		}()
	}
	wg.Wait()
}
