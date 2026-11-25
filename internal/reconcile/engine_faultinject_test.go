package reconcile

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// driftCtl converges observed to desired on each pass.
type driftCtl struct {
	desired  atomic.Int32
	observed atomic.Int32
}

func (d *driftCtl) Name() string { return "drift" }
func (d *driftCtl) Reconcile(context.Context) (Result, error) {
	d.observed.Store(d.desired.Load())
	return Result{}, nil
}

// Every event is dropped (closed stream), so only the periodic resync can repair drift.
func TestRun_DroppedEventsConvergeOnResync(t *testing.T) {
	c := &driftCtl{}
	e := NewEngine(testLogger(), c)
	events := make(chan docker.Event)
	close(events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interval := 50 * time.Millisecond
	go func() { _ = e.Run(ctx, events, interval) }()

	c.desired.Store(7)
	deadline := time.Now().Add(20 * interval)
	for c.observed.Load() != 7 {
		if time.Now().After(deadline) {
			t.Fatalf("observed = %d, want 7 within resync interval", c.observed.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
