package application

import (
	"context"
	"strings"
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
	c := New("migrate", &fakeStore{svc: oneShot("no")}, rt, WithReadyBudget(time.Second), WithCompletionBudget(150*time.Millisecond), WithStartupWatch(0))

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("error = nil, want a one-shot still running past the budget to be not Ready")
	}
	if cond := conditionOf(t, res); cond.Status == reconcile.ConditionTrue || cond.Reason != ReasonCompletionTimeout {
		t.Fatalf("condition = %+v, want not Ready with reason %s", cond, ReasonCompletionTimeout)
	}
	if rt.stopCalls != 0 {
		t.Fatalf("Stop calls = %d, want a still-running one-shot left running", rt.stopCalls)
	}
}

// A migration longer than the ready budget must not be killed and rerun.
func TestRunToCompletion_LongRunOutlivesReadyBudget(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitImg: "other:v1"}
	c := New("migrate", &fakeStore{svc: oneShot("no")}, rt, WithReadyBudget(100*time.Millisecond), WithStartupWatch(0))

	res, err := c.Reconcile(context.Background())
	if cond := conditionOf(t, res); err == nil || cond.Reason != ReasonRunningToComplete {
		t.Fatalf("first pass: err = %v condition = %+v, want not Ready with reason %s", err, cond, ReasonRunningToComplete)
	}
	if rt.stopCalls != 0 {
		t.Fatalf("Stop calls = %d, want the running migration left alone", rt.stopCalls)
	}

	target := replicaContainerName("migrate", "img:v1", "", 0)
	now := time.Now()
	rt.crashContainer(target, false, 0)
	rt.setExitState(target, &docker.ExitState{ExitCode: 0, StartedAt: now.Add(-90 * time.Second), FinishedAt: now})
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("after exit 0: error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue || cond.Reason != ReasonCompleted {
		t.Fatalf("after exit 0: condition = %+v, want Ready with reason %s", cond, ReasonCompleted)
	}
	if rt.startCalls != 1 {
		t.Fatalf("Start calls = %d, want 1 (the migration ran once)", rt.startCalls)
	}
}

func TestRunToCompletion_OverCompletionBudgetIsNotKilled(t *testing.T) {
	rt := newFakeRuntime(0)
	target := replicaContainerName("migrate", "img:v1", "", 0)
	rt.seed(target, true)
	rt.containers[target].Image = "img:v1"
	rt.setExitState(target, &docker.ExitState{Running: true, StartedAt: time.Now().Add(-90 * time.Second)})
	c := New("migrate", &fakeStore{svc: oneShot("no")}, rt, WithCompletionBudget(time.Minute), WithStartupWatch(0))

	res, err := c.Reconcile(context.Background())
	if cond := conditionOf(t, res); err == nil || cond.Status == reconcile.ConditionTrue || cond.Reason != ReasonCompletionTimeout {
		t.Fatalf("err = %v condition = %+v, want not Ready with reason %s", err, cond, ReasonCompletionTimeout)
	}
	if rt.stopCalls != 0 || rt.removeCalls != 0 || !rt.containers[target].Running {
		t.Fatalf("stop=%d remove=%d, want the over-budget one-shot left running", rt.stopCalls, rt.removeCalls)
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

func completedDependent() *store.DesiredService {
	return &store.DesiredService{
		Name: "myapp-web", AppID: "myapp", Image: "img:v1", Replicas: 1,
		DependsOn: []string{"migrate"},
		Health:    &store.ServiceHealth{DependsOnCompleted: []string{"migrate"}},
	}
}

func TestDependsOnCompleted_WaitsForExitZero(t *testing.T) {
	rt := newFakeRuntime(0)
	dep := replicaContainerName("myapp-migrate", "img:v1", "", 0)
	rt.seed(dep, true)
	rt.setExitState(dep, &docker.ExitState{Running: true, StartedAt: time.Now()})
	c := New("myapp-web", &fakeStore{svc: completedDependent()}, rt, WithStartupWatch(0))

	res, _ := c.Reconcile(context.Background())
	if cond := conditionOf(t, res); cond.Reason != reasonWaitingForCompletion || rt.createCalls != 0 {
		t.Fatalf("while the dependency runs: condition = %+v, creates = %d, want %s and no create", cond, rt.createCalls, reasonWaitingForCompletion)
	}

	rt.crashContainer(dep, false, 0)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("after exit 0: error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue || rt.createCalls == 0 {
		t.Fatalf("after exit 0: condition = %+v, creates = %d, want the dependent started", cond, rt.createCalls)
	}
}

func TestDependsOnCompleted_NonzeroExitBlocksWithReason(t *testing.T) {
	rt := newFakeRuntime(0)
	dep := replicaContainerName("myapp-migrate", "img:v1", "", 0)
	rt.seed(dep, false)
	rt.crashContainer(dep, false, 2)
	c := New("myapp-web", &fakeStore{svc: completedDependent()}, rt, WithStartupWatch(0))

	res, _ := c.Reconcile(context.Background())
	cond := conditionOf(t, res)
	if cond.Reason != reasonDependencyFailed || rt.createCalls != 0 || !strings.Contains(cond.Message, "exit code 2") {
		t.Fatalf("condition = %+v, creates = %d, want %s naming exit code 2 and no create", cond, rt.createCalls, reasonDependencyFailed)
	}
}

// A plain service_started dependency on a one-shot that already finished must not block forever.
func TestDependsOnStarted_CompletedDependencyCounts(t *testing.T) {
	rt := newFakeRuntime(0)
	dep := replicaContainerName("myapp-migrate", "img:v1", "", 0)
	rt.seed(dep, false)
	rt.crashContainer(dep, false, 0)
	svc := completedDependent()
	svc.Health = nil
	c := New("myapp-web", &fakeStore{svc: svc}, rt, WithStartupWatch(0))

	if _, err := c.Reconcile(context.Background()); err != nil || rt.createCalls == 0 {
		t.Fatalf("error = %v, creates = %d, want the dependent started", err, rt.createCalls)
	}
}

// timedRuntime reports exit times, as the local Docker client does.
type timedRuntime struct{ *exitOnStartRuntime }

func (timedRuntime) ReportsExitTimes() bool { return true }

// A created, never started container reports exit 0 with no start time: it did not complete.
func TestRunToCompletion_CreatedNeverStartedIsStarted(t *testing.T) {
	inner := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: 0, exitImg: "img:v1"}
	rt := timedRuntime{inner}
	target := replicaContainerName("migrate", "img:v1", "", 0)
	inner.seed(target, false)
	inner.containers[target].Image = "img:v1"
	inner.setExitState(target, &docker.ExitState{ExitCode: 0})
	c := New("migrate", &fakeStore{svc: oneShot("no")}, rt, WithStartupWatch(0))

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if inner.startCalls != 1 {
		t.Fatalf("Start calls = %d, want the never-started one-shot started once", inner.startCalls)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want Ready after it ran and exited 0", cond)
	}
}

func TestCompletedExit_RequiresStartAndFinishWhenReported(t *testing.T) {
	now := time.Now()
	timed := timedRuntime{&exitOnStartRuntime{fakeRuntime: newFakeRuntime(0)}}
	untimed := newFakeRuntime(0)
	cases := []struct {
		name string
		src  docker.ExitStateInspector
		es   *docker.ExitState
		want bool
	}{
		{"timed created", timed, &docker.ExitState{}, false},
		{"timed ran", timed, &docker.ExitState{StartedAt: now.Add(-time.Second), FinishedAt: now}, true},
		{"untimed transport", untimed, &docker.ExitState{}, true},
		{"nonzero", timed, &docker.ExitState{ExitCode: 1, StartedAt: now, FinishedAt: now}, false},
	}
	for _, tc := range cases {
		if got := CompletedExit(tc.src, tc.es); got != tc.want {
			t.Errorf("%s: CompletedExit = %v, want %v", tc.name, got, tc.want)
		}
	}
}
