package store

import (
	"context"
	"testing"
)

func TestSaveHeartbeatTokenResetsHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveHeartbeatToken(ctx, "mon_1", "h1", 10); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordHeartbeat(ctx, "mon_1", "fail", 20); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveHeartbeatToken(ctx, "mon_1", "h2", 30); err != nil {
		t.Fatal(err)
	}
	h, err := db.GetHeartbeat(ctx, "mon_1")
	if err != nil {
		t.Fatal(err)
	}
	if h.LastFail != 0 || h.ArmedAt != 30 || h.TokenHash != "h2" {
		t.Fatalf("rotation kept history: %+v", h)
	}
}
