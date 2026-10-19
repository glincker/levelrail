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

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testGuard(cfg GuardConfig) (*Guard, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	g := NewGuard(cfg)
	g.now = clk.now
	return g, clk
}

func TestGuardBreakerOpensBacksOffAndRecovers(t *testing.T) {
	g, clk := testGuard(GuardConfig{BreakerFailures: 3, BreakerBase: 10 * time.Second, BreakerMax: 40 * time.Second, ResourceBurst: 1000, ResourcePerSec: 1000})
	boom := errors.New("boom")
	for i := 0; i < 3; i++ {
		if !g.Before("app/a") {
			t.Fatalf("failure %d: breaker must still be closed", i)
		}
		g.After("app/a", boom, false)
	}
	if g.Before("app/a") {
		t.Fatal("breaker must be open after the threshold")
	}
	if st := g.Status(); st.OpenBreakers != 1 || st.Breakers[0].State != BreakerOpen || st.Breakers[0].LastError != "boom" {
		t.Fatalf("unexpected status: %+v", st)
	}
	clk.advance(11 * time.Second)
	if !g.Before("app/a") {
		t.Fatal("after the wait one trial run must be allowed")
	}
	g.After("app/a", boom, false)
	clk.advance(11 * time.Second)
	if g.Before("app/a") {
		t.Fatal("a failed trial must back off further (20s), not reopen after 10s")
	}
	clk.advance(10 * time.Second)
	if !g.Before("app/a") {
		t.Fatal("second trial must be allowed after the longer wait")
	}
	g.After("app/a", nil, false)
	if st := g.Status(); st.OpenBreakers != 0 || st.Breakers[0].State != BreakerClosed {
		t.Fatalf("a success must close the breaker: %+v", st)
	}
}

func TestGuardBackoffIsBounded(t *testing.T) {
	g, clk := testGuard(GuardConfig{BreakerFailures: 1, BreakerBase: time.Second, BreakerMax: 8 * time.Second, ResourceBurst: 1000, ResourcePerSec: 1000})
	for i := 0; i < 30; i++ {
		clk.advance(time.Hour)
		g.Before("x")
		g.After("x", errors.New("e"), false)
	}
	if wait := g.Status().Breakers[0].OpenUntil.Sub(clk.now()); wait > 8*time.Second {
		t.Fatalf("backoff %s exceeds the cap", wait)
	}
}

func TestGuardGlobalBudgetDefersThenRefills(t *testing.T) {
	g, clk := testGuard(GuardConfig{PassBurst: 3, PassesPerSec: 2})
	for i := 0; i < 3; i++ {
		if ok, _ := g.AllowPass(); !ok {
			t.Fatalf("pass %d inside the burst must be allowed", i)
		}
	}
	ok, wait := g.AllowPass()
	if ok || wait <= 0 || wait > time.Second {
		t.Fatalf("want a short deferral, got ok=%v wait=%s", ok, wait)
	}
	clk.advance(wait)
	if ok, _ := g.AllowPass(); !ok {
		t.Fatal("after the advised wait the pass must be allowed")
	}
	if st := g.Status(); st.PassesDeferred != 1 || st.RecentLimited != 1 {
		t.Fatalf("limit counters wrong: %+v", st)
	}
}

func TestGuardResourceRateLimit(t *testing.T) {
	g, clk := testGuard(GuardConfig{ResourceBurst: 2, ResourcePerSec: 1})
	for i := 0; i < 2; i++ {
		if !g.Before("a") {
			t.Fatalf("run %d inside the burst must pass", i)
		}
	}
	if g.Before("a") {
		t.Fatal("third immediate run must be throttled")
	}
	if !g.Before("b") {
		t.Fatal("limits are per resource")
	}
	clk.advance(2 * time.Second)
	if !g.Before("a") {
		t.Fatal("tokens must refill")
	}
}

func TestGuardResetClosesBreakers(t *testing.T) {
	g, _ := testGuard(GuardConfig{BreakerFailures: 1, ResourceBurst: 1000, ResourcePerSec: 1000})
	for _, n := range []string{"a", "b"} {
		g.Before(n)
		g.After(n, errors.New("e"), false)
	}
	if got := g.Reset("a"); got != 1 {
		t.Fatalf("reset a closed %d", got)
	}
	if !g.Before("a") || g.Before("b") {
		t.Fatal("only a should be closed")
	}
	if got := g.Reset(""); got != 1 {
		t.Fatalf("reset all closed %d", got)
	}
	if !g.Before("b") {
		t.Fatal("b must run after a full reset")
	}
}

type scriptedController struct {
	name  string
	calls atomic.Int64
	fn    func(n int64) error
}

func (s *scriptedController) Name() string { return s.name }
func (s *scriptedController) Reconcile(context.Context) (Result, error) {
	n := s.calls.Add(1)
	return Result{}, s.fn(n)
}

func quietEngine(g *Guard, cs ...Controller) *Engine {
	e := NewEngine(slog.New(slog.NewTextHandler(io.Discard, nil)), cs...)
	e.SetGuard(g)
	return e
}

func TestEnginePanicIsContainedAndTripsBreaker(t *testing.T) {
	bad := &scriptedController{name: "bad", fn: func(int64) error { panic("kaboom") }}
	good := &scriptedController{name: "good", fn: func(int64) error { return nil }}
	g := NewGuard(GuardConfig{BreakerFailures: 3, BreakerBase: time.Hour})
	e := quietEngine(g, bad, good)
	for i := 0; i < 10; i++ {
		e.ReconcileAll(context.Background())
	}
	if got := bad.calls.Load(); got != 3 {
		t.Fatalf("a panicking controller must stop after the threshold, ran %d times", got)
	}
	if got := good.calls.Load(); got != 10 {
		t.Fatalf("a healthy controller must keep running, ran %d times", got)
	}
	if st := g.Status(); st.OpenBreakers != 1 || st.Breakers[0].Panics != 3 {
		t.Fatalf("breaker state wrong: %+v", st)
	}
}

// TestEventChurnFuzz hammers the engine with random events and nudges: passes stay within the
// budget, a failing controller stays quiet, and the final desired state still converges.
func TestEventChurnFuzz(t *testing.T) {
	var desired, observed atomic.Int64
	converger := &scriptedController{name: "converger", fn: func(int64) error {
		observed.Store(desired.Load())
		return nil
	}}
	flaky := &scriptedController{name: "flaky", fn: func(int64) error { return errors.New("always fails") }}
	g := NewGuard(GuardConfig{PassBurst: 5, PassesPerSec: 50, BreakerFailures: 4, BreakerBase: time.Hour, ResourceBurst: 10000, ResourcePerSec: 10000})
	e := quietEngine(g, converger, flaky)

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan docker.Event, 64)
	done := make(chan struct{})
	go func() { _ = e.Run(ctx, events, time.Hour); close(done) }()

	seed := uint32(7)
	next := func(n uint32) uint32 { seed = seed*1664525 + 1013904223; return (seed >> 16) % n }
	start := time.Now()
	sent := 0
	for time.Since(start) < 300*time.Millisecond {
		desired.Add(1)
		if next(3) == 0 {
			e.Nudge()
		} else {
			select {
			case events <- docker.Event{ContainerName: "c"}:
			default:
			}
		}
		sent++
	}
	elapsed := time.Since(start)

	deadline := time.Now().Add(2 * time.Second)
	for observed.Load() != desired.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if observed.Load() != desired.Load() {
		t.Fatalf("desired state did not converge after the storm: %d vs %d", observed.Load(), desired.Load())
	}
	budget := int64(5+50*elapsed.Seconds()) + 8
	if got := converger.calls.Load(); got > budget {
		t.Fatalf("%d passes for %d triggers exceeds the budget %d", got, sent, budget)
	}
	if got := flaky.calls.Load(); got != 4 {
		t.Fatalf("a failing controller must be stopped by its breaker after 4 runs, ran %d", got)
	}
	if st := g.Status(); st.PassesDeferred == 0 {
		t.Fatalf("the storm must have been rate limited: %+v", st)
	}
}
