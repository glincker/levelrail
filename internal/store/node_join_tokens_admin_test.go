package store

import (
	"context"
	"testing"
	"time"
)

func TestRevokePendingNodeJoinTokens_LeavesUsedAndExpiredAlone(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	save := func(id, hash string, expires time.Time) {
		t.Helper()
		if err := db.SaveNodeJoinToken(ctx, NodeJoinToken{ID: id, TokenHash: hash, CreatedAt: now, ExpiresAt: expires}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	save("live", "h-live", now.Add(time.Hour))
	save("used", "h-used", now.Add(time.Hour))
	save("old", "h-old", now.Add(-time.Hour))
	if err := db.MarkNodeJoinTokenUsed(ctx, "used"); err != nil {
		t.Fatalf("mark used: %v", err)
	}

	pending, err := db.ListPendingNodeJoinTokens(ctx, now)
	if err != nil || len(pending) != 1 || pending[0].ID != "live" {
		t.Fatalf("pending = %+v, err = %v, want only live", pending, err)
	}
	n, err := db.RevokePendingNodeJoinTokens(ctx, "", now)
	if err != nil || n != 1 {
		t.Fatalf("revoked = %d, err = %v, want 1", n, err)
	}
	used, _ := db.GetNodeJoinTokenByHash(ctx, "h-used")
	if used.UsedAt == nil {
		t.Error("used token lost its used_at")
	}
	if left, _ := db.ListPendingNodeJoinTokens(ctx, now); len(left) != 0 {
		t.Errorf("pending after revoke = %d, want 0", len(left))
	}
}
