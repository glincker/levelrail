package reconcile

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// requeueingApp is the shape a readiness wait takes once it leaves the worker:
// each run probes once, reports not ready and asks to be looked at again.
type requeueingApp struct {
	name    string
	after   time.Duration
	runs    atomic.Int32
	active  atomic.Int32
	overlap atomic.Int32
}

func (r *requeueingApp) Name() string { return r.name }
func (r *requeueingApp) Reconcile(context.Context) (Result, error) {
	if r.active.Add(1) > 1 {
		r.overlap.Add(1)
	}
	defer r.active.Add(-1)
	r.runs.Add(1)
	return Result{
		Conditions:   []Condition{{Type: "Ready", Status: ConditionFalse, Reason: "ReadinessPending"}},
		RequeueAfter: r.after,
	}, nil
}

func runEngine(t *testing.T, e *Engine) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, nil, 20*time.Millisecond) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Run did not return after cancel")
		}
	})
}

// A never-ready app that requeues instead of blocking must neither starve 20
// healthy apps nor hot-loop, at any worker count including the default of 1.
func TestStarvation_RequeueingNeverReadyApp(t *testing.T) {
	t.Setenv(EnvRequeueMax, "200ms")
	for _, workers := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			bad := &requeueingApp{name: "app/bad", after: 20 * time.Millisecond}
			healthy := make([]*countingController, 20)
			for i := range healthy {
				healthy[i] = &countingController{name: fmt.Sprintf("app/h%02d", i)}
			}
			e := NewEngine(testLogger())
			e.SetWorkers(workers, time.Hour)
			e.SetKeyed(isApp)
			e.SetSource(func(context.Context) ([]Controller, error) {
				out := []Controller{bad}
				for _, h := range healthy {
					out = append(out, h)
				}
				return out, nil
			})
			runEngine(t, e)

			waitFor(t, 2*time.Second, "every healthy app reconciled", func() bool {
				for _, h := range healthy {
					if h.calls.Load() == 0 {
						return false
					}
				}
				return true
			})
			time.Sleep(500 * time.Millisecond)
			if n := bad.runs.Load(); n < 2 || n > 200 {
				t.Errorf("never-ready app ran %d times in about 0.5s, want a bounded requeue loop", n)
			}
			if bad.overlap.Load() != 0 {
				t.Error("never-ready app ran concurrently with itself")
			}
		})
	}
}

// With the readiness wait still on the worker, N>=2 workers keep healthy apps
// converging while one app holds its worker; at most one run per key.
func TestStarvation_BlockingNeverReadyAppWithPool(t *testing.T) {
	for _, workers := range []int{2, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			bad := &blockingApp{name: "app/bad", release: make(chan struct{})}
			defer close(bad.release)
			healthy := make([]*countingController, 20)
			for i := range healthy {
				healthy[i] = &countingController{name: fmt.Sprintf("app/h%02d", i)}
			}
			var visible atomic.Int32
			visible.Store(10)
			e := NewEngine(testLogger())
			e.SetWorkers(workers, time.Hour)
			e.SetKeyed(isApp)
			e.SetSource(func(context.Context) ([]Controller, error) {
				out := []Controller{bad}
				for _, h := range healthy[:visible.Load()] {
					out = append(out, h)
				}
				return out, nil
			})
			runEngine(t, e)

			waitFor(t, time.Second, "never-ready app in flight", func() bool { return bad.calls.Load() == 1 })
			visible.Store(20)
			e.Nudge()
			waitFor(t, 2*time.Second, "every healthy app reconciled, including ones created while bad is stuck", func() bool {
				for _, h := range healthy {
					if h.calls.Load() == 0 {
						return false
					}
				}
				return true
			})
			if got := bad.calls.Load(); got != 1 {
				t.Errorf("never-ready app started %d times while in flight, want 1", got)
			}
			if s := e.Stats().Queue; s == nil || s.InFlight > workers {
				t.Errorf("queue stats = %+v, want at most %d in flight", s, workers)
			}
		})
	}
}
