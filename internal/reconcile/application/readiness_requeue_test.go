package application

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const requeueDelay = 5 * time.Millisecond

func requeueDesired(image string, budget time.Duration) *store.DesiredService {
	return &store.DesiredService{
		Name: "web", Image: image, Port: 80, Replicas: 1, Strategy: strategyBlueGreen,
		Health: &store.ServiceHealth{
			Readiness:    &store.ServiceProbe{Path: "/healthz", Interval: 5 * time.Millisecond, Timeout: 50 * time.Millisecond},
			ReadyTimeout: budget,
		},
	}
}

// runOnce builds a fresh controller per run, as the reconcile source does every pass.
func runOnce(st *fakeStore, rt *fakeRuntime, tr *PendingReadiness) (reconcile.Result, error) {
	return New("web", st, rt, WithReadinessRequeue(tr, requeueDelay)).Reconcile(context.Background())
}

// runUntilSettled re-runs while the result is ReadinessPending, as the key-only requeue would.
func runUntilSettled(t *testing.T, st *fakeStore, rt *fakeRuntime, tr *PendingReadiness) (reconcile.Result, error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := runOnce(st, rt, tr)
		if len(res.Conditions) == 0 || res.Conditions[0].Reason != reasonReadinessPending {
			return res, err
		}
		if err != nil {
			t.Fatalf("pending run returned error %v, want nil so the breaker does not count it", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("candidate still pending after 5s")
		}
		time.Sleep(requeueDelay)
	}
}

func assertPending(t *testing.T, res reconcile.Result, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("pending run error = %v, want nil", err)
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != reasonReadinessPending {
		t.Fatalf("condition = %+v, want False/%s", cond, reasonReadinessPending)
	}
	if !res.KeyOnly || res.RequeueAfter != requeueDelay {
		t.Fatalf("result requeue = %v keyOnly=%v, want a fixed key-only %v", res.RequeueAfter, res.KeyOnly, requeueDelay)
	}
}

func isRunning(rt *fakeRuntime, name string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	cs := rt.containers[name]
	return cs != nil && cs.Running
}

func TestReadinessRequeue_PendingThenReadyCutsOverOnce(t *testing.T) {
	srv := healthyAfter(150 * time.Millisecond)
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := newFakeRuntime(serverPort(t, srv))
	rt.seed(v1, true)
	desired := requeueDesired("img:v2", time.Minute)
	desired.Hooks = &store.ServiceHooks{PreDeploy: "migrate"}
	st := &fakeStore{svc: desired}
	tr := NewPendingReadiness()

	res, err := runOnce(st, rt, tr)
	assertPending(t, res, err)
	if !running(rt, v1) || !running(rt, v2) {
		t.Fatalf("containers = %v, want the serving release kept beside the pending candidate", rt.names())
	}

	res, err = runUntilSettled(t, st, rt, tr)
	if err != nil || conditionOf(t, res).Status != reconcile.ConditionTrue {
		t.Fatalf("settled = %+v, %v, want Ready", res, err)
	}
	assertNames(t, "after cutover", rt.names(), []string{v2})
	if n := len(rt.execCalls); n != 1 {
		t.Fatalf("pre-deploy hook ran %d times, want once across requeued runs", n)
	}
	if got := tr.targets("web"); len(got) != 0 {
		t.Fatalf("tracker still holds %v after cutover", got)
	}
}

func TestReadinessRequeue_DeadlineFailsAndStopsCandidate(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := newFakeRuntime(serverPort(t, srv))
	rt.seed(v1, true)
	st := &fakeStore{svc: requeueDesired("img:v2", 100*time.Millisecond)}

	res, err := runUntilSettled(t, st, rt, NewPendingReadiness())
	if err == nil {
		t.Fatal("error = nil, want the readiness failure once the budget ran out")
	}
	if cond := conditionOf(t, res); cond.Reason != "ReadinessFailed" {
		t.Fatalf("reason = %q, want ReadinessFailed", cond.Reason)
	}
	if running(rt, v2) || !running(rt, v1) {
		t.Fatalf("v1 running=%v v2 running=%v, want the candidate stopped and the old release serving", running(rt, v1), running(rt, v2))
	}
}

// pinnedRequeueRuntime binds the pinned host port to srv so the candidate's probe reaches it.
func pinnedRequeueRuntime(t *testing.T, srv *httptest.Server, oldName string) (*fakeRuntime, int) {
	t.Helper()
	port := serverPort(t, srv)
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	rt.seed(oldName, true)
	ports := []docker.PortBinding{{ContainerPort: 80, HostPort: port}}
	rt.containers[oldName].Ports = ports
	// A restarted release republishes what it was created with.
	if rt.specPorts == nil {
		rt.specPorts = map[string][]docker.PortBinding{}
	}
	rt.specPorts[rt.containers[oldName].ID] = ports
	return rt, port
}

func pinnedRequeueDesired(image string, port int, budget time.Duration) *store.DesiredService {
	d := requeueDesired(image, budget)
	d.HostPort = &port
	return d
}

func TestReadinessRequeue_PinnedPortKeepsPreviousStoppedThenRestores(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt, port := pinnedRequeueRuntime(t, srv, v1)
	st := &fakeStore{svc: pinnedRequeueDesired("img:v2", port, 150*time.Millisecond)}
	tr := NewPendingReadiness()

	res, err := runOnce(st, rt, tr)
	assertPending(t, res, err)
	res, err = runOnce(st, rt, tr)
	assertPending(t, res, err)
	if running(rt, v1) || !running(rt, v2) {
		t.Fatalf("v1 running=%v v2 running=%v, want the previous release held stopped while the candidate is pending", running(rt, v1), running(rt, v2))
	}

	if _, err := runUntilSettled(t, st, rt, tr); err == nil {
		t.Fatal("error = nil, want the readiness failure")
	}
	if !running(rt, v1) {
		t.Fatal("previous release not restarted after the candidate failed")
	}
	if slices.Contains(rt.names(), v2) {
		t.Fatal("failed candidate still holds the pinned port")
	}
}

func TestReadinessRequeue_PinnedPortReadyRetiresPrevious(t *testing.T) {
	srv := healthyAfter(100 * time.Millisecond)
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt, port := pinnedRequeueRuntime(t, srv, v1)
	st := &fakeStore{svc: pinnedRequeueDesired("img:v2", port, time.Minute)}

	res, err := runUntilSettled(t, st, rt, NewPendingReadiness())
	if err != nil || conditionOf(t, res).Status != reconcile.ConditionTrue {
		t.Fatalf("settled = %+v, %v, want Ready", res, err)
	}
	assertNames(t, "after handoff", rt.names(), []string{v2})
}

func TestReadinessRequeue_NewerDesiredCancelsPendingCandidate(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	v3 := replicaContainerName("web", "img:v3", "", 0)
	rt, port := pinnedRequeueRuntime(t, srv, v1)
	st := &fakeStore{svc: pinnedRequeueDesired("img:v2", port, 150*time.Millisecond)}
	tr := NewPendingReadiness()

	res, err := runOnce(st, rt, tr)
	assertPending(t, res, err)

	st.svc = pinnedRequeueDesired("img:v3", port, 150*time.Millisecond)
	res, err = runOnce(st, rt, tr)
	assertPending(t, res, err)
	if running(rt, v2) || !running(rt, v3) || running(rt, v1) {
		t.Fatalf("v1=%v v2=%v v3=%v, want v2 cancelled and v3 pending with v1 held stopped", running(rt, v1), running(rt, v2), running(rt, v3))
	}

	if _, err := runUntilSettled(t, st, rt, tr); err == nil {
		t.Fatal("error = nil, want v3's readiness failure")
	}
	if !running(rt, v1) {
		t.Fatal("previous release not restored after the superseding candidate failed")
	}
}

func TestReadinessRequeue_CrashWhilePendingFailsWithoutRestart(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := newFakeRuntime(serverPort(t, srv))
	rt.seed(v1, true)
	st := &fakeStore{svc: requeueDesired("img:v2", time.Minute)}
	tr := NewPendingReadiness()

	res, err := runOnce(st, rt, tr)
	assertPending(t, res, err)
	rt.crashContainer(v2, true, 137)
	starts := rt.startCalls

	res, err = runOnce(st, rt, tr)
	if err == nil {
		t.Fatal("error = nil, want the crash reported as a readiness failure")
	}
	if cond := conditionOf(t, res); cond.Reason == reasonReadinessPending || cond.Status != reconcile.ConditionFalse {
		t.Fatalf("condition = %+v, want a crash failure", cond)
	}
	if rt.startCalls != starts {
		t.Fatal("crashed candidate was restarted inside the run that judged it")
	}
	if !running(rt, v1) {
		t.Fatal("serving release stopped")
	}
}

// A control plane restart loses the tracker; the wait resumes from the
// container's own start time instead of reporting RunningNotReady forever.
func TestReadinessRequeue_RestartResumesWaitFromStartTime(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := newFakeRuntime(serverPort(t, srv))
	rt.seed(v1, true)
	st := &fakeStore{svc: requeueDesired("img:v2", 200*time.Millisecond)}

	res, err := runOnce(st, rt, NewPendingReadiness())
	assertPending(t, res, err)
	rt.setExitState(v2, &docker.ExitState{Running: true, StartedAt: time.Now()})
	time.Sleep(time.Millisecond)

	restarted := NewPendingReadiness()
	res, err = runOnce(st, rt, restarted)
	assertPending(t, res, err)
	if _, err := runUntilSettled(t, st, rt, restarted); err == nil {
		t.Fatal("error = nil, want the resumed wait to fail at its deadline")
	}
	if running(rt, v2) {
		t.Fatal("candidate still running after the resumed wait failed")
	}
}

// Without the option the blocking wait is unchanged: one run, one verdict.
func TestReadinessRequeue_OffKeepsBlockingWait(t *testing.T) {
	srv := healthyAfter(50 * time.Millisecond)
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	res, err := New("web", &fakeStore{svc: requeueDesired("img:v2", time.Minute)}, rt).Reconcile(context.Background())
	if err != nil || conditionOf(t, res).Status != reconcile.ConditionTrue || res.KeyOnly {
		t.Fatalf("result = %+v, %v, want Ready from a single blocking run", res, err)
	}
}

// Rolling keeps the blocking wait: it retires one old replica per ready step.
func TestReadinessRequeue_RollingStaysBlocking(t *testing.T) {
	srv := healthyAfter(50 * time.Millisecond)
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	d := requeueDesired("img:v2", time.Minute)
	d.Strategy = strategyRolling
	res, err := runOnce(&fakeStore{svc: d}, rt, NewPendingReadiness())
	if err != nil || conditionOf(t, res).Status != reconcile.ConditionTrue {
		t.Fatalf("result = %+v, %v, want Ready from a single blocking run", res, err)
	}
}
