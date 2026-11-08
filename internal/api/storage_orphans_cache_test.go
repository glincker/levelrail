package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrphansReadKeyIsPerRouterAndForgettable(t *testing.T) {
	a, b := &Router{}, &Router{}
	if a.orphansReadKey() == b.orphansReadKey() {
		t.Fatal("routers must not share a cache key")
	}
	var calls atomic.Int32
	fn := func(context.Context) (any, error) { calls.Add(1); return 1, nil }
	ctx := context.Background()
	if _, err := a.expensiveRead(ctx, a.orphansReadKey(), fn); err != nil {
		t.Fatal(err)
	}
	if _, err := a.expensiveRead(ctx, a.orphansReadKey(), fn); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("second read must be cached, got %d runs", calls.Load())
	}
	// done closes before expires is set, so give the entry a moment to settle.
	time.Sleep(10 * time.Millisecond)
	a.forgetExpensiveRead(a.orphansReadKey())
	if _, err := a.expensiveRead(ctx, a.orphansReadKey(), fn); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("read after forget must recompute, got %d runs", calls.Load())
	}
}
