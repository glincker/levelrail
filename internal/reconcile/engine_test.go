package reconcile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// countingController records how many times Reconcile was called and can
// be told to fail on demand, so tests can assert Engine's orchestration
// behavior without needing a real resource behind it.
type countingController struct {
	name  string
	calls atomic.Int32
	err   error
}

func (c *countingController) Name() string { return c.name }

func (c *countingController) Reconcile(_ context.Context) (Result, error) {
	c.calls.Add(1)
	if c.err != nil {
		return Result{Conditions: []Condition{{Type: "Ready", Status: ConditionFalse, Reason: "Failed"}}}, c.err
	}
	return Result{Conditions: []Condition{{Type: "Ready", Status: ConditionTrue, Reason: "OK"}}}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// fakeStore is a hand-written fake Store, not a mocking framework, same
// pattern nginxdemo's tests use for docker.Runtime.
type fakeStore struct {
	mu       sync.Mutex
	upserts  int
	lastName string
	lastCond []Condition
	err      error
}

func (f *fakeStore) UpsertConditions(_ context.Context, name string, conditions []Condition) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	f.lastName = name
	f.lastCond = conditions
	return f.err
}

func (f *fakeStore) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.upserts
}

func TestEngine_ReconcileAll_RunsEveryController(t *testing.T) {
	a := &countingController{name: "a"}
	b := &countingController{name: "b"}
	e := NewEngine(testLogger(), a, b)

	e.ReconcileAll(context.Background())

	if got := a.calls.Load(); got != 1 {
		t.Errorf("controller a: got %d calls, want 1", got)
	}
	if got := b.calls.Load(); got != 1 {
		t.Errorf("controller b: got %d calls, want 1", got)
	}
}

func TestEngine_ReconcileAll_OneFailureDoesNotBlockOthers(t *testing.T) {
	failing := &countingController{name: "failing", err: errors.New("boom")}
	healthy := &countingController{name: "healthy"}
	e := NewEngine(testLogger(), failing, healthy)

	e.ReconcileAll(context.Background())

	if got := healthy.calls.Load(); got != 1 {
		t.Errorf("healthy controller: got %d calls, want 1: one controller failing must not block the others", got)
	}

	_, err := e.LastResult("failing")
	if err == nil {
		t.Error("expected LastResult to surface the failing controller's error")
	}
}

func TestEngine_LastResult_UnknownController(t *testing.T) {
	e := NewEngine(testLogger())
	result, err := e.LastResult("never-registered")
	if err != nil {
		t.Errorf("expected nil error for a controller that never ran, got %v", err)
	}
	if len(result.Conditions) != 0 {
		t.Errorf("expected zero-value Result, got %+v", result)
	}
}

func TestEngine_Run_ReactsToEventsAndTicks(t *testing.T) {
	c := &countingController{name: "c"}
	e := NewEngine(testLogger(), c)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	events := make(chan docker.Event)
	done := make(chan error, 1)
	go func() {
		done <- e.Run(ctx, events, 20*time.Millisecond)
	}()

	// One explicit event, on top of the initial reconcile and whatever
	// ticks land before the deadline.
	events <- docker.Event{Action: docker.EventDie, ContainerName: "whatever"}

	<-done

	if got := c.calls.Load(); got < 3 {
		t.Errorf("got %d reconciles (initial + event + at least one tick), want >= 3", got)
	}
}

func TestEngine_Run_ClosedEventChannelFallsBackToTicker(t *testing.T) {
	c := &countingController{name: "c"}
	e := NewEngine(testLogger(), c)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	events := make(chan docker.Event)
	close(events) // simulate the stream dying immediately

	err := e.Run(ctx, events, 15*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	if got := c.calls.Load(); got < 2 {
		t.Errorf("got %d reconciles after event channel closed, want the ticker to keep driving reconciles (>= 2)", got)
	}
}

func TestEngine_ReconcileAll_PersistsToStoreWhenSet(t *testing.T) {
	c := &countingController{name: "nginx-demo"}
	store := &fakeStore{}
	e := NewEngine(testLogger(), c)
	e.SetStore(store)

	e.ReconcileAll(context.Background())

	if got := store.calls(); got != 1 {
		t.Fatalf("expected store.UpsertConditions called once, got %d", got)
	}
	if store.lastName != "nginx-demo" {
		t.Errorf("store received controller name %q, want %q", store.lastName, "nginx-demo")
	}
	if len(store.lastCond) != 1 || store.lastCond[0].Reason != "OK" {
		t.Errorf("store received conditions %+v, want one condition with Reason=OK", store.lastCond)
	}
}

func TestEngine_ReconcileAll_NoStoreSet_DoesNotPanic(t *testing.T) {
	c := &countingController{name: "a"}
	e := NewEngine(testLogger(), c) // no SetStore call

	e.ReconcileAll(context.Background()) // must not panic on nil store

	if got := c.calls.Load(); got != 1 {
		t.Errorf("got %d calls, want 1", got)
	}
}

func TestEngine_ReconcileAll_StoreFailureDoesNotFailReconcile(t *testing.T) {
	c := &countingController{name: "a"}
	store := &fakeStore{err: errors.New("disk full")}
	e := NewEngine(testLogger(), c)
	e.SetStore(store)

	e.ReconcileAll(context.Background())

	// The controller's own Reconcile succeeded; a store failure must not
	// retroactively turn that into a recorded failure. Persisting status
	// is best-effort, not load-bearing for the reconcile's own success.
	_, err := e.LastResult("a")
	if err != nil {
		t.Errorf("LastResult error = %v, want nil: a store failure must not propagate as the controller's error", err)
	}
	if got := store.calls(); got != 1 {
		t.Errorf("expected store.UpsertConditions still attempted once despite returning an error, got %d calls", got)
	}
}

func TestEngine_ReconcileAll_RunsFixedAndDynamicControllers(t *testing.T) {
	fixed := &countingController{name: "fixed"}
	dynamic := &countingController{name: "dynamic"}
	e := NewEngine(testLogger(), fixed)
	e.SetSource(func(_ context.Context) ([]Controller, error) {
		return []Controller{dynamic}, nil
	})

	e.ReconcileAll(context.Background())

	if got := fixed.calls.Load(); got != 1 {
		t.Errorf("fixed controller: got %d calls, want 1", got)
	}
	if got := dynamic.calls.Load(); got != 1 {
		t.Errorf("dynamic controller: got %d calls, want 1", got)
	}
}

func TestEngine_ReconcileAll_SourceReflectsCurrentState(t *testing.T) {
	// A Source is called fresh every pass, not cached from the first
	// call: this is what lets an app created or deleted through the HTTP
	// API take effect on the very next reconcile with no restart.
	names := []string{"app-a"}
	calls := map[string]*countingController{"app-a": {name: "app-a"}}
	e := NewEngine(testLogger())
	e.SetSource(func(_ context.Context) ([]Controller, error) {
		controllers := make([]Controller, 0, len(names))
		for _, n := range names {
			c, ok := calls[n]
			if !ok {
				c = &countingController{name: n}
				calls[n] = c
			}
			controllers = append(controllers, c)
		}
		return controllers, nil
	})

	e.ReconcileAll(context.Background())
	names = []string{"app-a", "app-b"}
	e.ReconcileAll(context.Background())

	if got := calls["app-a"].calls.Load(); got != 2 {
		t.Errorf("app-a: got %d calls, want 2 (present both passes)", got)
	}
	if got := calls["app-b"].calls.Load(); got != 1 {
		t.Errorf("app-b: got %d calls, want 1 (only present on the second pass)", got)
	}
}

func TestEngine_ReconcileAll_SourceErrorSkipsDynamicPassButNotFixed(t *testing.T) {
	fixed := &countingController{name: "fixed"}
	e := NewEngine(testLogger(), fixed)
	e.SetSource(func(_ context.Context) ([]Controller, error) {
		return nil, errors.New("store unavailable")
	})

	e.ReconcileAll(context.Background())

	if got := fixed.calls.Load(); got != 1 {
		t.Errorf("fixed controller: got %d calls, want 1: a Source error must not block the fixed set", got)
	}
}
