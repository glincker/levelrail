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
