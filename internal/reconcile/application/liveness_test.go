package application

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// livenessProbeInterval is short enough that every pass in a test is
// due for a probe, paired with settle() between passes.
const livenessProbeInterval = time.Millisecond

func settle() { time.Sleep(2 * livenessProbeInterval) }

// livenessBackend stands in for one container's HTTP surface: it counts
// requests per path and can be switched between healthy, failing, and
// hung, the three states a liveness check has to tell apart.
type livenessBackend struct {
	srv *httptest.Server

	mu     sync.Mutex
	status int
	hang   bool
	calls  map[string]int
	block  chan struct{}
}

func newLivenessBackend(t *testing.T) *livenessBackend {
	t.Helper()
	b := &livenessBackend{status: http.StatusOK, calls: map[string]int{}, block: make(chan struct{})}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.calls[r.URL.Path]++
		status, hang := b.status, b.hang
		block := b.block
		b.mu.Unlock()
		if hang {
			<-block
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(func() {
		b.mu.Lock()
		close(b.block)
		b.hang = false
		b.mu.Unlock()
		b.srv.Close()
	})
	return b
}

func (b *livenessBackend) port(t *testing.T) int { return serverPort(t, b.srv) }

func (b *livenessBackend) setStatus(status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = status
}

func (b *livenessBackend) setHang(hang bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hang = hang
}

func (b *livenessBackend) callsFor(path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[path]
}

// livenessService builds a desired service with a liveness probe and,
// unless readiness is wanted too, nothing else that would probe.
func livenessService(failures int, timeout time.Duration) *store.DesiredService {
	return &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Liveness: &store.ServiceProbe{
			Path:     "/livez",
			Interval: livenessProbeInterval,
			Timeout:  timeout,
			Failures: failures,
		}},
	}
}

func TestController_Liveness_NotConfigured_NeverProbes(t *testing.T) {
	backend := newLivenessBackend(t)
	rt := newFakeRuntime(backend.port(t))
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "AlreadyRunning" {
		t.Errorf("condition = %+v, want Status=True Reason=AlreadyRunning (unchanged behavior with no liveness block)", cond)
	}
	if got := backend.callsFor("/livez"); got != 0 {
		t.Errorf("liveness requests = %d, want 0: probing must be fully opt-in", got)
	}
	if rt.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0", rt.stopCalls)
	}
}

func TestController_Liveness_Healthy_StaysReady(t *testing.T) {
	backend := newLivenessBackend(t)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(2, 500*time.Millisecond)
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "AlreadyRunning" {
		t.Errorf("condition = %+v, want Status=True Reason=AlreadyRunning", cond)
	}
	if got := backend.callsFor("/livez"); got != 1 {
		t.Errorf("liveness requests = %d, want 1", got)
	}
	if rt.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0: a healthy container must never be restarted", rt.stopCalls)
	}
}

func TestController_Liveness_BelowThreshold_DoesNotRestart(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(3, 500*time.Millisecond)
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)

	c := New("web", &fakeStore{svc: desired}, rt)
	var cond reconcile.Condition
	for i := 0; i < 2; i++ {
		result, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: Reconcile() error = %v, want nil: a failure under the threshold is not a reconcile failure", i, err)
		}
		cond = conditionOf(t, result)
		settle()
	}
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "LivenessDegraded" {
		t.Errorf("condition = %+v, want Status=True Reason=LivenessDegraded", cond)
	}
	if !strings.Contains(cond.Message, "(2/3)") {
		t.Errorf("condition message = %q, want it to report 2 of 3 failures", cond.Message)
	}
	if rt.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0 before the threshold is reached", rt.stopCalls)
	}
	state, _ := rt.InspectByName(context.Background(), target)
	if state == nil || !state.Running {
		t.Errorf("state = %+v, want the container still running", state)
	}
}

func TestController_Liveness_ThresholdReached_RestartsContainer(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(2, 500*time.Millisecond)
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)
	before, _ := rt.InspectByName(context.Background(), target)

	c := New("web", &fakeStore{svc: desired}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("first pass: Reconcile() error = %v", err)
	}
	settle()

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the liveness failure surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessFailedRestarting" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessFailedRestarting", cond)
	}
	if rt.stopCalls != 1 || rt.startCalls != 1 {
		t.Errorf("stopCalls = %d, startCalls = %d, want 1 and 1", rt.stopCalls, rt.startCalls)
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 {
		t.Errorf("createCalls = %d, removeCalls = %d, want 0 and 0: a liveness restart reuses the container, it does not replace it", rt.createCalls, rt.removeCalls)
	}
	after, _ := rt.InspectByName(context.Background(), target)
	if after == nil || !after.Running || after.ID != before.ID {
		t.Errorf("state = %+v, want the same container back up (was %+v)", after, before)
	}
}

// TestController_Liveness_HungContainer_Restarts is the scenario this
// whole check exists for: the container is running and accepting
// connections, so every other signal says healthy, but it never answers.
func TestController_Liveness_HungContainer_Restarts(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setHang(true)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(1, 30*time.Millisecond)
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the hung container reported")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessFailedRestarting" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessFailedRestarting", cond)
	}
	if rt.stopCalls != 1 {
		t.Errorf("stopCalls = %d, want 1", rt.stopCalls)
	}
	state, _ := rt.InspectByName(context.Background(), target)
	if state == nil || !state.Running {
		t.Errorf("state = %+v, want the restarted container running", state)
	}
}

// TestController_Liveness_RestartFails_RecoversOnNextPass is the
// half-succeeded case: the container is stopped but never starts again.
// Reconcile must report that rather than claim a successful restart,
// and the next pass must recover it through the ordinary restart path,
// since an interrupted operation has to converge on a later pass.
func TestController_Liveness_RestartFails_RecoversOnNextPass(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	rt.startErr = errors.New("no space left on device")
	desired := livenessService(1, 500*time.Millisecond)
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the failed restart surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessRestartFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessRestartFailed", cond)
	}
	state, _ := rt.InspectByName(context.Background(), target)
	if state == nil || state.Running {
		t.Fatalf("state = %+v, want the container stopped after the failed start", state)
	}

	backend.setStatus(http.StatusOK)
	rt.startErr = nil
	settle()

	result, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("recovery pass: Reconcile() error = %v", err)
	}
	cond = conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("recovery condition = %+v, want Status=True Reason=Deployed", cond)
	}
	state, _ = rt.InspectByName(context.Background(), target)
	if state == nil || !state.Running {
		t.Errorf("state = %+v, want the container recovered by the ordinary restart path", state)
	}
}

// TestController_Liveness_StopFails_NeverStarts is the other half of the
// failed-restart case: if the container cannot be stopped, Start must
// never run behind it, and the failure must still be reported.
func TestController_Liveness_StopFails_NeverStarts(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	rt.stopErr = errors.New("permission denied")
	desired := livenessService(1, 500*time.Millisecond)
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the failed stop surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessRestartFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessRestartFailed", cond)
	}
	if rt.startCalls != 0 {
		t.Errorf("startCalls = %d, want 0: Start must never follow a failed Stop", rt.startCalls)
	}
}

// TestController_Liveness_ProbeUnavailable_CountsNoFailure separates a
// probe that genuinely failed from one that could not run at all: a
// broken inspect says nothing about the app's health, so it must not
// bank a failure toward the restart threshold.
func TestController_Liveness_ProbeUnavailable_CountsNoFailure(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(2, 500*time.Millisecond)
	rt.seed(ContainerName("web", desired.Image, ""), true)
	// Call 1 is ensureReplicaRunning's own inspect, call 2 is the
	// liveness check's: only the latter is broken here.
	rt.inspectErrOnCall = 2

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the broken inspect surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "LivenessProbeUnavailable" {
		t.Errorf("condition = %+v, want Status=True Reason=LivenessProbeUnavailable: a probe that could not run is not evidence of an unhealthy app", cond)
	}
	if got := backend.callsFor("/livez"); got != 0 {
		t.Errorf("liveness requests = %d, want 0", got)
	}
	settle()

	result, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("second pass: Reconcile() error = %v", err)
	}
	cond = conditionOf(t, result)
	if !strings.Contains(cond.Message, "(1/2)") {
		t.Errorf("condition = %+v, want the first real failure counted as 1 of 2 (the unavailable pass banked nothing)", cond)
	}
	if rt.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0", rt.stopCalls)
	}
}

// TestController_Liveness_NoPublishedPort_ProbeUnavailable is the other
// cannot-run case: a running container Docker reports no port binding
// for has no address to probe.
func TestController_Liveness_NoPublishedPort_ProbeUnavailable(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := livenessService(1, 500*time.Millisecond)
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the unprobeable container surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "LivenessProbeUnavailable" {
		t.Errorf("condition = %+v, want Status=True Reason=LivenessProbeUnavailable", cond)
	}
	if rt.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0: never restart a container on the strength of a probe that never ran", rt.stopCalls)
	}
}

func TestController_Liveness_SuccessResetsFailureCount(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(3, 500*time.Millisecond)
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("failing pass: Reconcile() error = %v", err)
	}
	settle()

	backend.setStatus(http.StatusOK)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("healthy pass: Reconcile() error = %v", err)
	}
	settle()

	backend.setStatus(http.StatusServiceUnavailable)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("second failing pass: Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if !strings.Contains(cond.Message, "(1/3)") {
		t.Errorf("condition = %+v, want the count back at 1 of 3: only consecutive failures restart a container", cond)
	}
}

// TestController_Liveness_RespectsInterval pins the rate limit: Reconcile
// runs on every Docker event, so without one a 30s probe interval would
// become a probe per event.
func TestController_Liveness_RespectsInterval(t *testing.T) {
	backend := newLivenessBackend(t)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(3, 500*time.Millisecond)
	desired.Health.Liveness.Interval = time.Minute
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	for i := 0; i < 3; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("pass %d: Reconcile() error = %v", i, err)
		}
	}
	if got := backend.callsFor("/livez"); got != 1 {
		t.Errorf("liveness requests = %d over 3 passes, want 1: the spec'd interval, not the reconcile cadence, sets the probe rate", got)
	}
}

// TestController_Liveness_NotProbedOnDeployPass keeps the two probes in
// their own lanes: a pass that just started a container has already
// proven it ready, so liveness starts counting from there.
func TestController_Liveness_NotProbedOnDeployPass(t *testing.T) {
	backend := newLivenessBackend(t)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(3, 500*time.Millisecond)
	desired.Health.Readiness = &store.ServiceProbe{Path: "/ready", Interval: livenessProbeInterval, Timeout: 500 * time.Millisecond}

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if got := backend.callsFor("/ready"); got < 1 {
		t.Errorf("readiness requests = %d, want at least 1", got)
	}
	if got := backend.callsFor("/livez"); got != 0 {
		t.Errorf("liveness requests = %d, want 0 on the pass that deployed the container", got)
	}
}

func TestController_Liveness_MultipleReplicas_RestartsEachFailingReplica(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(1, 500*time.Millisecond)
	desired.Replicas = 2
	rt.seed(replicaContainerName("web", desired.Image, "", 0), true)
	rt.seed(replicaContainerName("web", desired.Image, "", 1), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the liveness failures surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessFailedRestarting" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessFailedRestarting", cond)
	}
	if rt.stopCalls != 2 {
		t.Errorf("stopCalls = %d, want 2: each replica is probed and restarted on its own", rt.stopCalls)
	}
}

// TestController_Liveness_RecreateStrategy_ProbesWhenConverged covers
// recreate's own already-converged early return, which does not go
// through finishReconcile.
func TestController_Liveness_RecreateStrategy_ProbesWhenConverged(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(1, 500*time.Millisecond)
	desired.Strategy = "recreate"
	rt.seed(ContainerName("web", desired.Image, ""), true)

	c := New("web", &fakeStore{svc: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the liveness failure surfaced")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessFailedRestarting" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessFailedRestarting", cond)
	}
}

// TestController_Liveness_SharedTracker_CountsAcrossRebuiltControllers
// pins the reason WithLivenessTracker exists: cmd/levelrail builds a
// fresh Controller per service on every reconcile pass, so failures only
// accumulate toward the threshold if the history outlives the
// controller that recorded them.
func TestController_Liveness_SharedTracker_CountsAcrossRebuiltControllers(t *testing.T) {
	backend := newLivenessBackend(t)
	backend.setStatus(http.StatusServiceUnavailable)
	rt := newFakeRuntime(backend.port(t))
	desired := livenessService(2, 500*time.Millisecond)
	rt.seed(ContainerName("web", desired.Image, ""), true)
	tracker := NewLivenessTracker()

	newPass := func() *Controller {
		return New("web", &fakeStore{svc: desired}, rt, WithLivenessTracker(tracker))
	}
	if _, err := newPass().Reconcile(context.Background()); err != nil {
		t.Fatalf("first pass: Reconcile() error = %v", err)
	}
	settle()

	result, err := newPass().Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the threshold reached across two controller instances")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "LivenessFailedRestarting" {
		t.Errorf("condition = %+v, want Status=False Reason=LivenessFailedRestarting", cond)
	}
	if rt.stopCalls != 1 {
		t.Errorf("stopCalls = %d, want 1", rt.stopCalls)
	}
}

func TestLivenessTracker(t *testing.T) {
	now := time.Now()

	t.Run("first probe is always due", func(t *testing.T) {
		tracker := NewLivenessTracker()
		if !tracker.due("web-abc", now, time.Minute) {
			t.Error("due() = false for a container never probed, want true")
		}
	})

	t.Run("interval gates the next probe", func(t *testing.T) {
		tracker := NewLivenessTracker()
		tracker.recordSuccess("web-abc", now)
		if tracker.due("web-abc", now.Add(29*time.Second), 30*time.Second) {
			t.Error("due() = true before the interval elapsed, want false")
		}
		if !tracker.due("web-abc", now.Add(30*time.Second), 30*time.Second) {
			t.Error("due() = false once the interval elapsed, want true")
		}
	})

	t.Run("failures accumulate and success clears them", func(t *testing.T) {
		tracker := NewLivenessTracker()
		if got := tracker.recordFailure("web-abc", now); got != 1 {
			t.Errorf("recordFailure() = %d, want 1", got)
		}
		if got := tracker.recordFailure("web-abc", now); got != 2 {
			t.Errorf("recordFailure() = %d, want 2", got)
		}
		tracker.recordSuccess("web-abc", now)
		if got := tracker.recordFailure("web-abc", now); got != 1 {
			t.Errorf("recordFailure() after a success = %d, want 1", got)
		}
	})

	t.Run("reset clears failures and restarts the interval", func(t *testing.T) {
		tracker := NewLivenessTracker()
		tracker.recordFailure("web-abc", now)
		tracker.reset("web-abc", now)
		if tracker.due("web-abc", now, time.Minute) {
			t.Error("due() = true right after reset, want false")
		}
		if got := tracker.recordFailure("web-abc", now); got != 1 {
			t.Errorf("recordFailure() after reset = %d, want 1", got)
		}
	})

	oldContainer := ContainerName("web", "img:v1", "")
	newContainer := ContainerName("web", "img:v2", "")

	t.Run("retain drops containers that no longer exist", func(t *testing.T) {
		tracker := NewLivenessTracker()
		tracker.recordFailure(oldContainer, now)
		tracker.recordFailure(newContainer, now)
		tracker.retain("web", []string{newContainer})
		if got := tracker.recordFailure(oldContainer, now); got != 1 {
			t.Errorf("recordFailure() for a dropped container = %d, want 1 (its history is gone)", got)
		}
		if got := tracker.recordFailure(newContainer, now); got != 2 {
			t.Errorf("recordFailure() for a retained container = %d, want 2 (its history is kept)", got)
		}
	})

	t.Run("retain leaves another service's history alone", func(t *testing.T) {
		// One tracker is shared by every service's controller, so a pass
		// for "web" must not touch "web-worker" (which prefix-matches).
		tracker := NewLivenessTracker()
		other := ContainerName("web-worker", "img:v1", "")
		tracker.recordFailure(other, now)
		tracker.retain("web", []string{newContainer})
		if got := tracker.recordFailure(other, now); got != 2 {
			t.Errorf("recordFailure() for another service's container = %d, want 2 (untouched)", got)
		}
	})

	t.Run("resetAll clears every container at once", func(t *testing.T) {
		tracker := NewLivenessTracker()
		tracker.recordFailure(oldContainer, now)
		tracker.recordFailure(newContainer, now)
		tracker.resetAll("web", []string{oldContainer, newContainer}, now)
		for _, name := range []string{oldContainer, newContainer} {
			if got := tracker.recordFailure(name, now); got != 1 {
				t.Errorf("recordFailure(%q) after resetAll = %d, want 1", name, got)
			}
		}
	})
}
