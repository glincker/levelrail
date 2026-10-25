package reconcile

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func isApp(name string) bool { return strings.HasPrefix(name, "app/") }

// blockingApp holds its reconcile until release closes or ctx ends, then fails
// like an app whose readiness never passes.
type blockingApp struct {
	name      string
	release   chan struct{}
	calls     atomic.Int32
	cancelled atomic.Int32
}

func (b *blockingApp) Name() string { return b.name }
func (b *blockingApp) Reconcile(ctx context.Context) (Result, error) {
	b.calls.Add(1)
	select {
	case <-b.release:
	case <-ctx.Done():
		b.cancelled.Add(1)
		return Result{}, ctx.Err()
	}
	return Result{}, errors.New("readiness failed")
}

func newPooledEngine(t *testing.T, workers int, keyTimeout time.Duration, ctrls func() []Controller) *Engine {
	t.Helper()
	e := NewEngine(testLogger())
	e.SetWorkers(workers, keyTimeout)
	e.SetKeyed(isApp)
	e.SetSource(func(context.Context) ([]Controller, error) { return ctrls(), nil })
	t.Cleanup(func() { e.stopQueue(0) })
	return e
}

func waitFor(t *testing.T, bound time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(bound)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("not within %v: %s", bound, what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestKeyedPool_SlowAppDoesNotDelayOthers(t *testing.T) {
	slow := &blockingApp{name: "app/a", release: make(chan struct{})}
	defer close(slow.release)
	fast := make([]*countingController, 5)
	for i := range fast {
		fast[i] = &countingController{name: fmt.Sprintf("app/%c", 'b'+i)}
	}
	ctrls := func() []Controller {
		out := []Controller{slow}
		for _, f := range fast {
			out = append(out, f)
		}
		return out
	}
	e := newPooledEngine(t, 4, time.Hour, ctrls)

	start := time.Now()
	e.ReconcileAll(context.Background())
	if d := time.Since(start); d > time.Second {
		t.Fatalf("pass waited %v for keyed controllers", d)
	}
	waitFor(t, time.Second, "apps b..f reconciled while a blocks", func() bool {
		for _, f := range fast {
			if f.calls.Load() == 0 {
				return false
			}
		}
		return true
	})
	// A second pass while a is still stuck must reach b..f again and not start a twice.
	e.ReconcileAll(context.Background())
	waitFor(t, time.Second, "second pass reached b..f", func() bool {
		for _, f := range fast {
			if f.calls.Load() < 2 {
				return false
			}
		}
		return true
	})
	if got := slow.calls.Load(); got != 1 {
		t.Fatalf("slow app started %d times while in flight, want 1", got)
	}
	// A fast run counts its call before its worker clears the in-flight slot.
	waitFor(t, time.Second, "a in flight and queued once", func() bool {
		s := e.Stats().Queue
		return s != nil && s.InFlight == 1 && s.Depth == 1
	})
}

func TestKeyedPool_KeyTimeoutFreesWorker(t *testing.T) {
	stuck := []*blockingApp{
		{name: "app/a", release: make(chan struct{})},
		{name: "app/b", release: make(chan struct{})},
	}
	late := &countingController{name: "app/c"}
	e := newPooledEngine(t, 2, 50*time.Millisecond, func() []Controller {
		return []Controller{stuck[0], stuck[1], late}
	})
	e.ReconcileAll(context.Background())
	waitFor(t, 2*time.Second, "app c ran after both workers hit the time limit", func() bool {
		return late.calls.Load() == 1
	})
	for _, s := range stuck {
		if s.cancelled.Load() != 1 {
			t.Errorf("%s: cancelled %d times, want 1", s.name, s.cancelled.Load())
		}
	}
	if _, err := e.LastResult("app/a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("app/a last error = %v, want deadline exceeded", err)
	}
}

// exclusiveApp fails the test if two workers ever run the same name at once.
type exclusiveApp struct {
	name    string
	active  atomic.Int32
	overlap atomic.Int32
	total   *atomic.Int32
	peak    *atomic.Int32
}

func (x *exclusiveApp) Name() string { return x.name }
func (x *exclusiveApp) Reconcile(context.Context) (Result, error) {
	if x.active.Add(1) > 1 {
		x.overlap.Add(1)
	}
	n := x.total.Add(1)
	for {
		p := x.peak.Load()
		if n <= p || x.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(time.Duration(len(x.name)%3) * time.Millisecond)
	x.total.Add(-1)
	x.active.Add(-1)
	return Result{}, nil
}

func TestKeyedPool_SameKeyNeverRunsTwiceAtOnce(t *testing.T) {
	var total, peak atomic.Int32
	apps := make([]*exclusiveApp, 6)
	for i := range apps {
		apps[i] = &exclusiveApp{name: fmt.Sprintf("app/%d%s", i, strings.Repeat("x", i)), total: &total, peak: &peak}
	}
	const workers = 4
	e := newPooledEngine(t, workers, time.Hour, func() []Controller {
		out := make([]Controller, len(apps))
		for i, a := range apps {
			out[i] = a
		}
		return out
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				e.ReconcileAll(context.Background())
			}
		}()
	}
	wg.Wait()
	waitFor(t, 5*time.Second, "queue drained", func() bool {
		s := e.Stats().Queue
		return s != nil && s.Depth == 0 && s.InFlight == 0
	})
	for _, a := range apps {
		if a.overlap.Load() != 0 {
			t.Errorf("%s ran concurrently with itself %d times", a.name, a.overlap.Load())
		}
	}
	if p := peak.Load(); p > workers {
		t.Errorf("peak concurrency %d exceeds %d workers", p, workers)
	}
}

// orderedController appends its name to a shared log on every reconcile.
type orderedController struct {
	name string
	mu   *sync.Mutex
	log  *[]string
}

func (o *orderedController) Name() string { return o.name }
func (o *orderedController) Reconcile(context.Context) (Result, error) {
	o.mu.Lock()
	*o.log = append(*o.log, o.name)
	o.mu.Unlock()
	return Result{}, nil
}

func TestKeyedPool_OneWorkerKeepsSequentialOrder(t *testing.T) {
	var mu sync.Mutex
	var log []string
	mk := func(name string) Controller { return &orderedController{name: name, mu: &mu, log: &log} }
	fixed := mk("fixed/x")
	dynamic := []Controller{mk("app/a"), mk("app/b"), mk("database/d"), mk("app/c"), mk("ingress")}
	e := NewEngine(testLogger(), fixed)
	e.SetWorkers(1, 0)
	e.SetKeyed(isApp)
	e.SetSource(func(context.Context) ([]Controller, error) { return dynamic, nil })
	e.ReconcileAll(context.Background())
	e.ReconcileAll(context.Background())

	want := []string{"fixed/x", "app/a", "app/b", "database/d", "app/c", "ingress"}
	want = append(want, want...)
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(log, want) {
		t.Fatalf("order = %v, want %v", log, want)
	}
	if e.Stats().Queue != nil {
		t.Fatal("one worker must not start the keyed pool")
	}
}

func TestKeyedPool_NonKeyedStayInlineAndOrdered(t *testing.T) {
	var mu sync.Mutex
	var log []string
	mk := func(name string) Controller { return &orderedController{name: name, mu: &mu, log: &log} }
	e := newPooledEngine(t, 4, time.Hour, func() []Controller {
		return []Controller{mk("app/a"), mk("database/d"), mk("ingress")}
	})
	e.ReconcileAll(context.Background())
	waitFor(t, time.Second, "app/a ran", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(log, "app/a")
	})
	mu.Lock()
	defer mu.Unlock()
	inline := slices.DeleteFunc(slices.Clone(log), isApp)
	if !slices.Equal(inline, []string{"database/d", "ingress"}) {
		t.Fatalf("inline order = %v", inline)
	}
}

func TestKeyedPool_RemovedAppIsCancelled(t *testing.T) {
	gone := &blockingApp{name: "app/gone", release: make(chan struct{})}
	defer close(gone.release)
	var present atomic.Bool
	present.Store(true)
	e := newPooledEngine(t, 2, time.Hour, func() []Controller {
		if present.Load() {
			return []Controller{gone}
		}
		return nil
	})
	e.ReconcileAll(context.Background())
	waitFor(t, time.Second, "app started", func() bool { return gone.calls.Load() == 1 })
	present.Store(false)
	e.ReconcileAll(context.Background())
	waitFor(t, time.Second, "deleted app's reconcile cancelled", func() bool { return gone.cancelled.Load() == 1 })
}

func TestKeyedPool_ShutdownDrainsWithoutLeaks(t *testing.T) {
	before := runtime.NumGoroutine()
	slow := &blockingApp{name: "app/a", release: make(chan struct{})}
	defer close(slow.release)
	e := NewEngine(testLogger())
	e.SetWorkers(4, time.Hour)
	e.SetKeyed(isApp)
	e.SetDrainTimeout(50 * time.Millisecond)
	e.SetSource(func(context.Context) ([]Controller, error) { return []Controller{slow}, nil })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, nil, time.Hour) }()
	waitFor(t, time.Second, "app started", func() bool { return slow.calls.Load() == 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the drain limit")
	}
	if slow.cancelled.Load() != 1 {
		t.Fatal("in-flight reconcile was not cancelled at the drain limit")
	}
	waitFor(t, time.Second, "goroutines back to baseline", func() bool { return runtime.NumGoroutine() <= before })
}

func TestWorkersFromEnv(t *testing.T) {
	for in, want := range map[string]int{"": 1, "0": 1, "-3": 1, "x": 1, "4": 4, "1000": maxWorkers} {
		if got := WorkersFromEnv(func(string) string { return in }); got != want {
			t.Errorf("WorkersFromEnv(%q) = %d, want %d", in, got, want)
		}
	}
	if got := KeyTimeoutFromEnv(func(string) string { return "" }); got != defaultKeyTimeout {
		t.Errorf("KeyTimeoutFromEnv default = %v", got)
	}
	if got := KeyTimeoutFromEnv(func(string) string { return "90s" }); got != 90*time.Second {
		t.Errorf("KeyTimeoutFromEnv(90s) = %v", got)
	}
}
