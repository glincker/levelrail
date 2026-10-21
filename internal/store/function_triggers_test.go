package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFunctionInvocationQueueLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	inv := FunctionInvocation{ID: "inv_1", Function: "hello", Source: "api", Method: "POST", Path: "/", Body: []byte("x"), MaxAttempts: 3, NextAttemptAt: now, CreatedAt: now}

	if err := db.EnqueueFunctionInvocation(ctx, inv, 1); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	inv2 := inv
	inv2.ID = "inv_2"
	if err := db.EnqueueFunctionInvocation(ctx, inv2, 1); !errors.Is(err, ErrFunctionQueueFull) {
		t.Fatalf("second enqueue err = %v, want queue full", err)
	}

	got, ok, err := db.ClaimFunctionInvocation(ctx, now)
	if err != nil || !ok || got.ID != "inv_1" || got.Attempts != 1 || got.Status != FunctionInvocationRunning {
		t.Fatalf("claim = %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, _ := db.ClaimFunctionInvocation(ctx, now); ok {
		t.Fatal("running call must not be claimed twice")
	}

	if err := db.SettleFunctionInvocation(ctx, "inv_1", FunctionInvocationDead, now, 500, "boom", now); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := db.RetryFunctionInvocation(ctx, "inv_1", now); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := db.RetryFunctionInvocation(ctx, "inv_1", now); !errors.Is(err, ErrFunctionInvocationNotDead) {
		t.Fatalf("retry of queued call err = %v", err)
	}
	counts, err := db.CountFunctionInvocations(ctx, "hello")
	if err != nil || counts[FunctionInvocationQueued] != 1 {
		t.Fatalf("counts = %v err=%v", counts, err)
	}
}

func TestFunctionInvocationDedupe(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	inv := FunctionInvocation{ID: "a", Function: "hello", Source: "webhook", DedupeKey: "sig", MaxAttempts: 1, NextAttemptAt: now, CreatedAt: now}
	if err := db.EnqueueFunctionInvocation(ctx, inv, 0); err != nil {
		t.Fatal(err)
	}
	inv.ID = "b"
	if err := db.EnqueueFunctionInvocation(ctx, inv, 0); !errors.Is(err, ErrFunctionInvocationDuplicate) {
		t.Fatalf("replayed signature err = %v, want duplicate", err)
	}
}
