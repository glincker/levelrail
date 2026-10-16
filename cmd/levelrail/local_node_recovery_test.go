package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBootstrapLocalNode_RecoversOfflineNodeOnRestart(t *testing.T) {
	tests := []struct {
		name       string
		status     store.NodeStatus
		lastSeen   time.Duration
		wantStatus store.NodeStatus
	}{
		{name: "offline after a long restart gap comes back online", status: store.NodeStatusOffline, lastSeen: -2 * time.Hour, wantStatus: store.NodeStatusOnline},
		{name: "already online stays online", status: store.NodeStatusOnline, lastSeen: -time.Minute, wantStatus: store.NodeStatusOnline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })

			const id = "local_test"
			seen := time.Now().Add(tt.lastSeen)
			if err := db.SaveNode(ctx, store.Node{ID: id, Name: "box", Status: store.NodeStatusOnline, LastSeenAt: &seen, CreatedAt: seen, UpdatedAt: seen}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpdateNodeStatus(ctx, id, tt.status); err != nil {
				t.Fatal(err)
			}

			if err := bootstrapLocalNode(ctx, db, id); err != nil {
				t.Fatalf("bootstrapLocalNode() error = %v", err)
			}
			got, err := db.GetNode(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.LastSeenAt == nil || time.Since(*got.LastSeenAt) > time.Minute {
				t.Errorf("last seen = %v, want it refreshed to now", got.LastSeenAt)
			}
		})
	}
}

func TestMarkLocalNodeOnline_OnlyWritesOnChange(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const id = "local_test"
	now := time.Now()
	if err := db.SaveNode(ctx, store.Node{ID: id, Name: "box", Status: store.NodeStatusOnline, LastSeenAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := markLocalNodeOnline(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	events, err := db.ListNodeStatusEvents(ctx, id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("status events = %d, want none when already online", len(events))
	}

	if err := db.UpdateNodeStatus(ctx, id, store.NodeStatusOffline); err != nil {
		t.Fatal(err)
	}
	if err := markLocalNodeOnline(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetNode(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.NodeStatusOnline {
		t.Errorf("status = %q, want online after recovery", got.Status)
	}
}
