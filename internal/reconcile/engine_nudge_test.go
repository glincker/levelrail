package reconcile

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestNudgeController_ClosesBreakerAndQueuesPass(t *testing.T) {
	e := NewEngine(testLogger())
	for i := 0; i < 50; i++ {
		e.guard.Before("application/web")
		e.guard.After("application/web", context.DeadlineExceeded, false)
	}
	e.NudgeController("application/web")
	if !e.guard.Before("application/web") {
		t.Fatal("breaker still open after NudgeController")
	}
	if len(e.nudge) != 1 {
		t.Fatal("no pass queued")
	}
}

type countCtl struct{ n atomic.Int32 }

func (c *countCtl) Name() string { return "count" }
func (c *countCtl) Reconcile(context.Context) (Result, error) {
	c.n.Add(1)
	return Result{}, nil
}

func waitPasses(t *testing.T, c *countCtl, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for c.n.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("passes = %d, want at least %d", c.n.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRun_BootPassesFireAfterStart(t *testing.T) {
	c := &countCtl{}
	e := NewEngine(testLogger(), c)
	e.SetBootPasses([]time.Duration{20 * time.Millisecond, 60 * time.Millisecond}, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.Run(ctx, nil, time.Hour) }()
	waitPasses(t, c, 3)
}

func TestRun_FirstEventSchedulesLaterPass(t *testing.T) {
	c := &countCtl{}
	e := NewEngine(testLogger(), c)
	e.SetBootPasses(nil, 30*time.Millisecond)
	events := make(chan docker.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.Run(ctx, events, time.Hour) }()
	waitPasses(t, c, 1)
	events <- docker.Event{}
	// Event pass plus the debounced pass that sees ports published after it.
	waitPasses(t, c, 3)
}
