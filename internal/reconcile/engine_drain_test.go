package reconcile

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type slowController struct {
	started  chan struct{}
	release  chan struct{}
	cutShort atomic.Bool
}

func (c *slowController) Name() string { return "slow" }

func (c *slowController) Reconcile(ctx context.Context) (Result, error) {
	close(c.started)
	select {
	case <-c.release:
	case <-ctx.Done():
		c.cutShort.Store(true)
	}
	return Result{}, nil
}

func TestEngine_Run_PassFinishesAfterShutdownSignal(t *testing.T) {
	c := &slowController{started: make(chan struct{}), release: make(chan struct{})}
	e := NewEngine(slog.New(slog.DiscardHandler), c)
	e.SetDrainTimeout(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, nil, time.Hour) }()

	<-c.started
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a pass was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(c.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the pass finished")
	}
	if c.cutShort.Load() {
		t.Fatal("pass was cancelled by the shutdown signal")
	}
}

func TestEngine_Run_PassCutAtDrainLimit(t *testing.T) {
	c := &slowController{started: make(chan struct{}), release: make(chan struct{})}
	e := NewEngine(slog.New(slog.DiscardHandler), c)
	e.SetDrainTimeout(50 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, nil, time.Hour) }()

	<-c.started
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the drain limit")
	}
	if !c.cutShort.Load() {
		t.Fatal("pass was not cancelled at the drain limit")
	}
}
