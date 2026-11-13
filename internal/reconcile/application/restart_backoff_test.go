package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRestartBackoff_Delays(t *testing.T) {
	b := NewRestartBackoff(5*time.Second, 40*time.Second, 10*time.Minute)
	now := time.Now()
	tests := []struct {
		restarts int
		want     time.Duration
	}{{1, 5 * time.Second}, {2, 10 * time.Second}, {3, 20 * time.Second}, {4, 40 * time.Second}, {5, 40 * time.Second}}
	for _, tt := range tests {
		b.recordRestart("web-abc", now)
		got, n := b.remaining("web-abc", now)
		if got != tt.want || n != tt.restarts {
			t.Errorf("after %d restarts: remaining = %s (n=%d), want %s", tt.restarts, got, n, tt.want)
		}
	}
	if got, _ := b.remaining("web-abc", now.Add(41*time.Second)); got != 0 {
		t.Errorf("remaining after delay elapsed = %s, want 0", got)
	}
}

func TestRestartBackoff_ResetsAfterStableRun(t *testing.T) {
	b := NewRestartBackoff(5*time.Second, time.Minute, 10*time.Minute)
	now := time.Now()
	b.recordRestart("web-abc", now)
	b.recordRestart("web-abc", now)
	b.observeRunning("web-abc", now.Add(11*time.Minute))
	b.recordRestart("web-abc", now.Add(11*time.Minute))
	if got, n := b.remaining("web-abc", now.Add(11*time.Minute)); n != 1 || got != 5*time.Second {
		t.Errorf("after reset: remaining = %s (n=%d), want 5s (n=1)", got, n)
	}
}

func TestRestartBackoff_NilAndDisabledNeverWait(t *testing.T) {
	var nilB *RestartBackoff
	nilB.recordRestart("x", time.Now())
	if got, _ := nilB.remaining("x", time.Now()); got != 0 {
		t.Errorf("nil backoff remaining = %s", got)
	}
	off := NewRestartBackoff(0, time.Minute, time.Minute)
	off.recordRestart("x", time.Now())
	if got, _ := off.remaining("x", time.Now()); got != 0 {
		t.Errorf("disabled backoff remaining = %s", got)
	}
}

// A crashing container is restarted once, then held in CrashLoopBackOff
// instead of being restarted on every die event.
func TestController_Reconcile_CrashLoopBackOff_HoldsRepeatedRestarts(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)
	backoff := NewRestartBackoff(time.Hour, time.Hour, time.Hour)

	c := New("web", &fakeStore{svc: desired}, rt, WithRestartBackoff(backoff))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if rt.startCalls != 1 {
		t.Fatalf("startCalls after first pass = %d, want 1", rt.startCalls)
	}

	rt.crashContainer(target, false, 1)
	result, err := New("web", &fakeStore{svc: desired}, rt, WithRestartBackoff(backoff)).Reconcile(context.Background())
	if err == nil {
		t.Fatal("second Reconcile() error = nil, want a backoff error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != reasonCrashLoopBackOff {
		t.Errorf("condition = %+v, want False/%s", cond, reasonCrashLoopBackOff)
	}
	if rt.startCalls != 1 {
		t.Errorf("startCalls = %d, want 1: a backed-off container must not be restarted", rt.startCalls)
	}
}

// A candidate that exits during its first readiness wait is kept (stopped,
// logs intact) and the next pass reports CrashLoopBackOff, not the same
// ExitedDuringReadiness forever.
func TestController_Reconcile_CrashDuringReadiness_ThenCrashLoopBackOff(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	target := ContainerName("web", desired.Image, "")
	time.AfterFunc(30*time.Millisecond, func() { rt.crashContainer(target, false, 1) })
	backoff := NewRestartBackoff(time.Hour, time.Hour, time.Hour)

	first, err := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second), WithRestartBackoff(backoff)).Reconcile(context.Background())
	if err == nil {
		t.Fatal("first Reconcile() error = nil, want the crash surfaced")
	}
	if cond := conditionOf(t, first); cond.Reason != "ExitedDuringReadiness" {
		t.Fatalf("first condition = %+v, want ExitedDuringReadiness", cond)
	}
	if state, _ := rt.InspectByName(context.Background(), target); state == nil || state.Running {
		t.Fatalf("failed candidate = %+v, want kept and stopped", state)
	}

	pass := func() reconcile.Result {
		res, _ := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second), WithRestartBackoff(backoff)).Reconcile(context.Background())
		return res
	}
	pass() // the one allowed restart, which crashes again
	starts := rt.startCalls
	if cond := conditionOf(t, pass()); cond.Status != reconcile.ConditionFalse || cond.Reason != reasonCrashLoopBackOff {
		t.Errorf("condition = %+v, want False/%s", cond, reasonCrashLoopBackOff)
	}
	if rt.startCalls != starts {
		t.Errorf("startCalls = %d, want %d: a backed-off container must not be restarted", rt.startCalls, starts)
	}
}

func TestRestartBackoffBreakerQuarantinesAndResumes(t *testing.T) {
	b := NewRestartBackoff(time.Second, time.Minute, time.Hour)
	b.SetBreaker(3, 0.5)
	b.rnd = func() float64 { return 1 }
	now := time.Now()
	b.recordRestart("web-5222ac0e", now)
	if wait, _ := b.remaining("web-5222ac0e", now); wait != 1500*time.Millisecond {
		t.Fatalf("first delay = %v, want 1.5s with full jitter", wait)
	}
	b.recordRestart("web-5222ac0e", now)
	if held, _ := b.Quarantined("web-5222ac0e"); held {
		t.Fatal("quarantined before the threshold")
	}
	b.recordRestart("web-5222ac0e", now)
	if held, n := b.Quarantined("web-5222ac0e"); !held || n != 3 {
		t.Fatalf("Quarantined = %v, %d, want held after 3 restarts", held, n)
	}
	// A long quiet spell must not release a quarantined container on its own.
	b.observeRunning("web-5222ac0e", now.Add(24*time.Hour))
	b.recordRestart("web-5222ac0e", now.Add(24*time.Hour))
	if held, _ := b.Quarantined("web-5222ac0e"); !held {
		t.Fatal("breaker released without a resume")
	}
	if got := b.Resume("web"); got != 1 {
		t.Fatalf("Resume = %d, want 1", got)
	}
	if held, _ := b.Quarantined("web-5222ac0e"); held {
		t.Fatal("still quarantined after resume")
	}
}

func TestRestartBackoffBreakerOffByDefault(t *testing.T) {
	b := NewRestartBackoff(time.Second, time.Minute, time.Hour)
	for range 50 {
		b.recordRestart("web-5222ac0e", time.Now())
	}
	if held, _ := b.Quarantined("web-5222ac0e"); held {
		t.Fatal("breaker must be off until SetBreaker")
	}
}

func TestController_Reconcile_QuarantinedContainerStaysStopped(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, AppID: "myapp"}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)
	backoff := NewRestartBackoff(time.Nanosecond, time.Nanosecond, time.Hour)
	backoff.SetBreaker(1, 0)
	backoff.recordRestart(target, time.Now())

	result, err := New("web", &fakeStore{svc: desired}, rt, WithRestartBackoff(backoff)).Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a quarantine error")
	}
	if cond := conditionOf(t, result); cond.Reason != reasonQuarantined {
		t.Errorf("reason = %q, want %q", cond.Reason, reasonQuarantined)
	}
	if rt.startCalls != 0 {
		t.Fatalf("startCalls = %d, want 0: a quarantined container must stay stopped", rt.startCalls)
	}

	backoff.Resume("web")
	if _, err := New("web", &fakeStore{svc: desired}, rt, WithRestartBackoff(backoff)).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() after resume error = %v", err)
	}
	if rt.startCalls != 1 {
		t.Fatalf("startCalls after resume = %d, want 1", rt.startCalls)
	}
}
