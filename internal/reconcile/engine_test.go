package reconcile

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
