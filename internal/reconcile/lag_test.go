package reconcile

import (
	"context"
	"errors"
	"testing"
	"time"
)

type lagCtl struct {
	name string
	err  error
}

func (c *lagCtl) Name() string { return c.name }
func (c *lagCtl) Reconcile(context.Context) (Result, error) {
	return Result{}, c.err
}

func TestLagGrowsUntilReconcileRuns(t *testing.T) {
	ok := &lagCtl{name: "application/web"}
	bad := &lagCtl{name: "mesh/hub", err: errors.New("down")}
	e := NewEngine(testLogger(), ok, bad)
	clock := time.Unix(1000, 0)
	e.now = func() time.Time { return clock }

	e.Nudge()
	clock = clock.Add(90 * time.Second)
	if got := e.Lag(); got["application"] != 90*time.Second || got["mesh"] != 90*time.Second {
		t.Fatalf("before pass: %v", got)
	}
	e.ReconcileAll(context.Background())
	got := e.Lag()
	if got["application"] != 0 {
		t.Fatalf("converged reconciler still lagging: %v", got)
	}
	if got["mesh"] != 0 {
		t.Fatalf("failing reconciler ran, so it must not report lag: %v", got)
	}
}

func TestLagIgnoresErroringControllerButKeepsUnrun(t *testing.T) {
	bad := &lagCtl{name: "application/crash", err: errors.New("boom")}
	e := NewEngine(testLogger(), bad)
	clock := time.Unix(1000, 0)
	e.now = func() time.Time { return clock }
	for i := 0; i < 3; i++ {
		e.Nudge()
		e.ReconcileAll(context.Background())
		clock = clock.Add(time.Minute)
	}
	if got := e.Lag()["application"]; got != 0 {
		t.Fatalf("erroring controller reports growing lag: %v", got)
	}
	e.Nudge()
	clock = clock.Add(2 * time.Minute)
	if got := e.Lag()["application"]; got != 2*time.Minute {
		t.Fatalf("unrun controller must lag: %v", got)
	}
}
