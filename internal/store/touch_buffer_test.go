package store

import (
	"context"
	"testing"
	"time"
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

func TestTouchBufferCoalescesSessionActivity(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetSessionMethod(ctx, "sess_buf", "usr_buf", "password", time.Unix(100, 0)); err != nil {
		t.Skipf("session fixture unavailable: %v", err)
	}
	for i := range 3 {
		if err := db.TouchSessionActivity(ctx, "sess_buf", "usr_buf", time.Unix(int64(200+i), 0)); err != nil {
			t.Fatalf("TouchSessionActivity() error = %v", err)
		}
	}
	db.touches.mu.Lock()
	pending := len(db.touches.sessions)
	db.touches.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pending sessions = %d, want 1", pending)
	}
	if err := db.FlushTouches(ctx); err != nil {
		t.Fatalf("FlushTouches() error = %v", err)
	}
	var seen int64
	if err := db.QueryRowContext(ctx, `SELECT last_seen_at FROM session_activity WHERE session_id = ?`, "sess_buf").Scan(&seen); err != nil || seen != 202 {
		t.Fatalf("last_seen_at = %d, err = %v, want 202", seen, err)
	}
}
