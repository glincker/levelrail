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

func oneShot(restart string) *store.DesiredService {
	return &store.DesiredService{Name: "migrate", Image: "img:v1", Replicas: 1, Health: &store.ServiceHealth{Restart: restart}}
}

func gatedOpts() []Option {
	return []Option{WithStabilityGate(deploysafety.Effective{SuccessThreshold: 1, StableWindow: 300 * time.Millisecond}, nil), WithGateTick(20 * time.Millisecond), WithStartupWatch(time.Second)}
}

// A one-shot that exits 0 inside the stable window is a successful deploy,
// and a later pass leaves it stopped instead of running it again.
func TestRunToCompletion_ExitZeroDeploysAndIsNotRestarted(t *testing.T) {
	for _, restart := range []string{"no", "on-failure", "on-failure:3"} {
		t.Run(restart, func(t *testing.T) {
			rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: 0, exitImg: "img:v1"}
			c := New("migrate", &fakeStore{svc: oneShot(restart)}, rt, gatedOpts()...)

			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("first deploy error = %v, want success once the one-shot exits 0", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("first deploy condition = %+v, want Ready", cond)
			}
			starts := rt.startCalls
			for pass := 0; pass < 3; pass++ {
				res, err = c.Reconcile(context.Background())
				if err != nil {
					t.Fatalf("pass %d: error = %v", pass, err)
				}
				if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue || cond.Reason != ReasonCompleted {
					t.Fatalf("pass %d: condition = %+v, want Ready with reason %s", pass, cond, ReasonCompleted)
				}
			}
			if rt.startCalls != starts {
				t.Fatalf("Start calls = %d after steady passes, want %d (a completed one-shot must not rerun)", rt.startCalls, starts)
			}
		})
	}
}

func TestRunToCompletion_NonzeroExitFailsDeploy(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: 3, exitImg: "img:v1"}
	c := New("migrate", &fakeStore{svc: oneShot("no")}, rt, gatedOpts()...)

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("error = nil, want a nonzero exit to fail the deploy")
	}
	if cond := conditionOf(t, res); cond.Status == reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want not Ready", cond)
	}
}

func TestRunToCompletion_NeverFinishingFailsWithinBudget(t *testing.T) {
	// exitImg never matches, so the container stays running.
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitImg: "other:v1"}
	c := New("migrate", &fakeStore{svc: oneShot("no")}, rt, WithReadyBudget(150*time.Millisecond), WithStartupWatch(0))

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("error = nil, want a one-shot still running past the budget to fail")
	}
	if cond := conditionOf(t, res); cond.Status == reconcile.ConditionTrue || cond.Reason != ReasonCompletionTimeout {
		t.Fatalf("condition = %+v, want not Ready with reason %s", cond, ReasonCompletionTimeout)
	}
}

func TestRunToCompletion_RecreateStrategyKeepsCompletedContainer(t *testing.T) {
	rt := newFakeRuntime(0)
	target := replicaContainerName("migrate", "img:v1", "", 0)
	rt.seed(target, false)
	rt.containers[target].Image = "img:v1"
	rt.setExitState(target, &docker.ExitState{ExitCode: 0, StartedAt: time.Now().Add(-time.Minute), FinishedAt: time.Now()})
	svc := oneShot("no")
	svc.Strategy = strategyRecreate
	c := New("migrate", &fakeStore{svc: svc}, rt)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Reason != ReasonCompleted {
		t.Fatalf("condition = %+v, want reason %s", cond, ReasonCompleted)
	}
	if rt.createCalls != 0 || rt.startCalls != 0 {
		t.Fatalf("create=%d start=%d, want a completed one-shot left alone", rt.createCalls, rt.startCalls)
	}
}

// The default (no explicit restart:) keeps today's crash handling: exit 0 is not done.
func TestRunToCompletion_UnsetRestartStillRestartsExitZero(t *testing.T) {
	rt := newFakeRuntime(0)
	target := replicaContainerName("migrate", "img:v1", "", 0)
	rt.seed(target, false)
	rt.containers[target].Image = "img:v1"
	rt.setExitState(target, &docker.ExitState{ExitCode: 0, StartedAt: time.Now().Add(-time.Minute), FinishedAt: time.Now()})
	c := New("migrate", &fakeStore{svc: oneShot("")}, rt, WithStartupWatch(0))

	_, _ = c.Reconcile(context.Background())
	if rt.startCalls == 0 {
		t.Fatal("Start not called, want an unset restart policy to keep restarting an exited container")
	}
}
