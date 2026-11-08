package store

import (
	"context"
	"testing"
)

func TestTouchBufferCoalescesUntilFlush(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveAPIToken(ctx, testToken("tok_buf", "hash-buf")); err != nil {
		t.Fatalf("SaveAPIToken() error = %v", err)
	}
	for range 5 {
		if err := db.TouchAPITokenLastUsed(ctx, "tok_buf"); err != nil {
			t.Fatalf("TouchAPITokenLastUsed() error = %v", err)
		}
	}
	db.touches.mu.Lock()
	pending := len(db.touches.tokens)
	db.touches.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending tokens = %d, want 1 coalesced entry", pending)
	}
	if err := db.FlushTouches(ctx); err != nil {
		t.Fatalf("FlushTouches() error = %v", err)
	}
	got, err := db.GetAPITokenByID(ctx, "tok_buf")
	if err != nil || got.LastUsedAt == nil {
		t.Fatalf("after flush LastUsedAt = %v, err = %v", got, err)
	}
}
