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

func TestLagGrowsUntilSuccessfulReconcile(t *testing.T) {
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
	if got["mesh"] != 90*time.Second {
		t.Fatalf("failing reconciler must keep lagging: %v", got)
	}
}
