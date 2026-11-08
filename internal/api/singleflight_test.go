package api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestExpensiveReadCollapsesConcurrentCalls(t *testing.T) {
	g := newFlightGroup(2)
	var runs atomic.Int32
	fn := func(context.Context) (any, error) {
		if runs.Add(1) < 0 {
			return nil, context.Canceled
		}
		time.Sleep(30 * time.Millisecond)
		return "v", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := g.do(context.Background(), "k", time.Minute, fn); err != nil || v != "v" {
				t.Errorf("got %v %v", v, err)
			}
		}()
	}
	wg.Wait()
	if _, _ = g.do(context.Background(), "k", time.Minute, fn); runs.Load() != 1 {
		t.Fatalf("runs = %d, want 1", runs.Load())
	}
}

func TestExpensiveReadDoesNotCacheErrors(t *testing.T) {
	g := newFlightGroup(1)
	var runs atomic.Int32
	fn := func(context.Context) (any, error) {
		if runs.Add(1) == 1 {
			return nil, context.DeadlineExceeded
		}
		return "ok", nil
	}
	if _, err := g.do(context.Background(), "k", time.Minute, fn); err == nil {
		t.Fatal("want error")
	}
	if v, err := g.do(context.Background(), "k", time.Minute, fn); err != nil || v != "ok" {
		t.Fatalf("got %v %v", v, err)
	}
}
