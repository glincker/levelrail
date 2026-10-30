package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func portlessWorker() *store.DesiredService {
	return &store.DesiredService{Name: "web", Image: "img:v1", Replicas: 1}
}

func exitedReason(reason string) bool {
	return reason == deploysafety.ReasonExitedDuringReadiness || reason == deploysafety.ReasonExitedDuringWindow
}

// A port-less service has no probe, so "running" is its whole readiness: a
// restart that exits again at once must not be reported Ready.
func TestExitedReadiness_RestartedPortlessExitIsNotReady(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		opts     []Option
	}{
		{name: "exit 0 with stable window", exitCode: 0, opts: []Option{WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: 300 * time.Millisecond}, nil), WithGateTick(20 * time.Millisecond), WithStartupWatch(time.Second)}},
		{name: "exit 2 with stable window", exitCode: 2, opts: []Option{WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: 300 * time.Millisecond}, nil), WithGateTick(20 * time.Millisecond), WithStartupWatch(time.Second)}},
		{name: "exit 0 with no gate and startup watch off", exitCode: 0, opts: []Option{WithStartupWatch(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: tt.exitCode, exitImg: "img:v1"}
			target := replicaContainerName("web", "img:v1", "", 0)
			rt.seed(target, false)
			rt.containers[target].Image = "img:v1"
			rt.crashContainer(target, false, tt.exitCode)
			c := New("web", &fakeStore{svc: portlessWorker()}, rt, tt.opts...)

			// Each pass is one turn of a restart loop: none may read as Ready.
			for pass := 0; pass < 3; pass++ {
				res, err := c.Reconcile(context.Background())
				if err == nil {
					t.Fatalf("pass %d: Reconcile error = nil, want the exit to fail readiness", pass)
				}
				cond := conditionOf(t, res)
				if cond.Status == reconcile.ConditionTrue || !exitedReason(cond.Reason) {
					t.Fatalf("pass %d: condition = %+v, want not Ready with an exit reason", pass, cond)
				}
			}
		})
	}
}

// Docker's container list can lag an exit a real inspect already sees.
func TestExitedReadiness_SteadyStateListLagIsNotReady(t *testing.T) {
	rt := newFakeRuntime(0)
	target := replicaContainerName("web", "img:v1", "", 0)
	rt.seed(target, true)
	rt.setExitState(target, &docker.ExitState{ExitCode: 0, StartedAt: time.Now().Add(-time.Minute), FinishedAt: time.Now()})
	c := New("web", &fakeStore{svc: portlessWorker()}, rt)

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile error = nil, want the exited container to be not Ready")
	}
	if cond := conditionOf(t, res); cond.Status == reconcile.ConditionTrue || !exitedReason(cond.Reason) {
		t.Fatalf("condition = %+v, want not Ready with an exit reason", cond)
	}
}

func TestExitedReadiness_HealthyPortlessStaysReady(t *testing.T) {
	gate := []Option{WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: 200 * time.Millisecond}, nil), WithGateTick(20 * time.Millisecond), WithStartupWatch(time.Second)}
	tests := []struct {
		name    string
		seed    func(rt *exitOnStartRuntime, target string)
		opts    []Option
		maxWait time.Duration
	}{
		{name: "already running", seed: func(rt *exitOnStartRuntime, target string) {
			rt.seed(target, true)
			rt.setExitState(target, &docker.ExitState{Running: true, StartedAt: time.Now().Add(-time.Minute)})
		}, opts: gate, maxWait: time.Second},
		{name: "restarted and stays up", seed: func(rt *exitOnStartRuntime, target string) {
			rt.seed(target, false)
			rt.crashContainer(target, false, 1)
		}, opts: gate, maxWait: 3 * time.Second},
		{name: "fresh deploy stays up", seed: func(*exitOnStartRuntime, string) {}, opts: gate, maxWait: 3 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitImg: "never"}
			target := replicaContainerName("web", "img:v1", "", 0)
			tt.seed(rt, target)
			c := New("web", &fakeStore{svc: portlessWorker()}, rt, tt.opts...)
			start := time.Now()
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if took := time.Since(start); took > tt.maxWait {
				t.Fatalf("Reconcile took %s, want under %s", took, tt.maxWait)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition = %+v, want Ready", cond)
			}
		})
	}
}

// A port-less release that exits at once fails the deploy inside the stable
// window and the previous release keeps serving.
func TestExitedReadiness_PortlessDeployExitFailsDeploy(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitImg: "img:v2"}
	old := replicaContainerName("web", "img:v1", "", 0)
	rt.seed(old, true)
	rt.setExitState(old, &docker.ExitState{Running: true, StartedAt: time.Now().Add(-time.Hour)})
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Strategy: "blue-green", Replicas: 1}
	window := 2 * time.Second
	c := New("web", &fakeStore{svc: desired}, rt,
		WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: window}, nil), WithGateTick(20*time.Millisecond), WithStartupWatch(time.Second))

	start := time.Now()
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile error = nil, want the deploy to fail")
	}
	if took := time.Since(start); took > window {
		t.Fatalf("deploy failed after %s, want inside the %s window", took, window)
	}
	if cond := conditionOf(t, res); cond.Status == reconcile.ConditionTrue || !exitedReason(cond.Reason) {
		t.Fatalf("condition = %+v, want not Ready with an exit reason", cond)
	}
	if cs := rt.containers[old]; cs == nil || !cs.Running {
		t.Fatal("previous release stopped, want it still serving")
	}
}

// A worker that crashes in steady state is restarted without a full stable
// window and without failing the deploy that already succeeded.
func TestExitedReadiness_SteadyStateCrashIsNotARollout(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: 2, exitImg: "img:v1"}
	target := replicaContainerName("web", "img:v1", "", 0)
	rt.seed(target, false)
	rt.containers[target].Image = "img:v1"
	rt.crashContainer(target, false, 2)
	rec := &fakeGateRecorder{}
	c := New("web", &fakeStore{svc: portlessWorker()}, rt,
		WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: 3 * time.Second}, nil),
		WithGateTick(20*time.Millisecond), WithStartupWatch(0), WithGateRecorder(rec))

	start := time.Now()
	res, err := c.Reconcile(context.Background())
	if took := time.Since(start); took > time.Second {
		t.Fatalf("Reconcile took %s, want a restart not to wait out the 3s stable window", took)
	}
	if cond := conditionOf(t, res); err == nil || cond.Status == reconcile.ConditionTrue || !exitedReason(cond.Reason) {
		t.Fatalf("err = %v condition = %+v, want not Ready with an exit reason", err, cond)
	}
	if len(rec.reasons) != 0 {
		t.Fatalf("rollout failure reasons = %v, want none for a steady-state crash", rec.reasons)
	}
}

func TestExitedReadiness_FirstDeployInstantExitFails(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: 2, exitImg: "img:v1"}
	rec := &fakeGateRecorder{}
	c := New("web", &fakeStore{svc: portlessWorker()}, rt,
		WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: 300 * time.Millisecond}, nil),
		WithGateTick(20*time.Millisecond), WithStartupWatch(0), WithGateRecorder(rec))

	res, err := c.Reconcile(context.Background())
	if cond := conditionOf(t, res); err == nil || cond.Status == reconcile.ConditionTrue {
		t.Fatalf("err = %v condition = %+v, want the first deploy to fail", err, cond)
	}
	if len(rec.reasons) == 0 {
		t.Fatal("no rollout failure reason recorded, want the first deploy marked failed")
	}
}
