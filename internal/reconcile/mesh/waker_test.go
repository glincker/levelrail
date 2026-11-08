package mesh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

type scheduled struct {
	d time.Duration
	f func()
}

// fakeWaker hands every scheduled timer to the test instead of a real clock.
func fakeWaker(target func(), window time.Duration) (*Waker, chan scheduled) {
	ch := make(chan scheduled, 64)
	w := NewWaker(target, func() time.Duration { return window })
	w.after = func(d time.Duration, f func()) { ch <- scheduled{d: d, f: f} }
	return w, ch
}

func next(t *testing.T, ch chan scheduled) scheduled {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no pass was scheduled")
		return scheduled{}
	}
}

func TestWakerSchedulesOnePassAfterWindow(t *testing.T) {
	var passes atomic.Int32
	w, ch := fakeWaker(func() { passes.Add(1) }, 750*time.Millisecond)

	w.Wake()
	s := next(t, ch)
	if s.d != 750*time.Millisecond {
		t.Errorf("scheduled after %v, want the 750ms window", s.d)
	}
	s.f()
	if got := passes.Load(); got != 1 {
		t.Errorf("passes = %d, want 1", got)
	}
}

func TestWakerCollapsesJoinStormIntoOnePass(t *testing.T) {
	var passes atomic.Int32
	w, ch := fakeWaker(func() { passes.Add(1) }, time.Second)

	for range 20 {
		w.Wake()
	}
	s := next(t, ch)
	select {
	case extra := <-ch:
		t.Fatalf("a 20 node burst scheduled a second pass after %v", extra.d)
	default:
	}
	s.f()
	if got := passes.Load(); got != 1 {
		t.Errorf("passes = %d, want 1 for the whole burst", got)
	}

	// After the pass fires, a later join must schedule again.
	w.Wake()
	next(t, ch).f()
	if got := passes.Load(); got != 2 {
		t.Errorf("passes = %d, want 2 after a later join", got)
	}
}

// countingController records how many reconcile passes ran.
type countingController struct{ n atomic.Int32 }

func (c *countingController) Name() string { return "mesh" }
func (c *countingController) Reconcile(context.Context) (reconcile.Result, error) {
	c.n.Add(1)
	return reconcile.Result{}, nil
}

func TestWakerRunsEnginePassWithinBound(t *testing.T) {
	ctrl := &countingController{}
	engine := reconcile.NewEngine(nil, ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// An hour-long resync proves the pass came from the wake, not the ticker.
	go func() { _ = engine.Run(ctx, nil, time.Hour) }()
	waitFor(t, func() bool { return ctrl.n.Load() >= 1 })

	w := NewWaker(engine.Nudge, func() time.Duration { return 10 * time.Millisecond })
	w.Wake()
	waitFor(t, func() bool { return ctrl.n.Load() >= 2 })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("no reconcile pass within 5s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
