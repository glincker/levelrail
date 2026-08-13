package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func testJoinToken() NodeJoinToken {
	now := time.Now()
	return NodeJoinToken{
		ID:        "njt_1",
		TokenHash: "hash-abc",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
}

func TestSaveAndGetNodeJoinTokenByHash(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := testJoinToken()
	if err := db.SaveNodeJoinToken(ctx, want); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	got, err := db.GetNodeJoinTokenByHash(ctx, "hash-abc")
	if err != nil {
		t.Fatalf("GetNodeJoinTokenByHash() error = %v", err)
	}
	if got.ID != want.ID || got.TokenHash != want.TokenHash {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.UsedAt != nil {
		t.Errorf("got.UsedAt = %v, want nil for a freshly saved token", got.UsedAt)
	}
}

func TestGetNodeJoinTokenByHash_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetNodeJoinTokenByHash(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNodeJoinTokenNotFound) {
		t.Errorf("GetNodeJoinTokenByHash() error = %v, want ErrNodeJoinTokenNotFound", err)
	}
}

func TestMarkNodeJoinTokenUsed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNodeJoinToken(ctx, testJoinToken()); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}
	if err := db.MarkNodeJoinTokenUsed(ctx, "njt_1"); err != nil {
		t.Fatalf("MarkNodeJoinTokenUsed() error = %v", err)
	}

	got, err := db.GetNodeJoinTokenByHash(ctx, "hash-abc")
	if err != nil {
		t.Fatalf("GetNodeJoinTokenByHash() error = %v", err)
	}
	if got.UsedAt == nil {
		t.Error("got.UsedAt = nil, want a timestamp after MarkNodeJoinTokenUsed")
	}
}

func TestMarkNodeJoinTokenUsed_AlreadyUsed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNodeJoinToken(ctx, testJoinToken()); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}
	if err := db.MarkNodeJoinTokenUsed(ctx, "njt_1"); err != nil {
		t.Fatalf("first MarkNodeJoinTokenUsed() error = %v", err)
	}

	err := db.MarkNodeJoinTokenUsed(ctx, "njt_1")
	if !errors.Is(err, ErrNodeJoinTokenAlreadyUsed) {
		t.Errorf("second MarkNodeJoinTokenUsed() error = %v, want ErrNodeJoinTokenAlreadyUsed", err)
	}
}

func TestMarkNodeJoinTokenUsed_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.MarkNodeJoinTokenUsed(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNodeJoinTokenNotFound) {
		t.Errorf("MarkNodeJoinTokenUsed() error = %v, want ErrNodeJoinTokenNotFound", err)
	}
}

// TestMarkNodeJoinTokenUsed_ConcurrentCallers_OnlyOneSucceeds is the
// same concurrency-safety proof TestCreateAdminUser_ConcurrentCallers_
// OnlySucceeds (admin_test.go) already established for a different
// single-use resource: the conditional UPDATE, not a caller-side
// check-then-act, is what has to decide between racing exchange
// attempts on the same join token.
func TestMarkNodeJoinTokenUsed_ConcurrentCallers_OnlyOneSucceeds(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNodeJoinToken(ctx, testJoinToken()); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	results := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = db.MarkNodeJoinTokenUsed(ctx, "njt_1")
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrNodeJoinTokenAlreadyUsed) {
			t.Errorf("unexpected error from a losing caller: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d, want exactly 1 (single-use token exchanged by exactly one caller)", successes)
	}
}
