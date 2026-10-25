package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToNodeResourceOfflineSince(t *testing.T) {
	seen := time.Now().Add(-time.Minute)
	off := toNodeResource(store.Node{ID: "n", Status: store.NodeStatusOffline, LastSeenAt: &seen})
	if off.OfflineSince == nil || !off.OfflineSince.Equal(seen) {
		t.Fatalf("offline node OfflineSince = %v, want %v", off.OfflineSince, seen)
	}
	on := toNodeResource(store.Node{ID: "n", Status: store.NodeStatusOnline, LastSeenAt: &seen})
	if on.OfflineSince != nil {
		t.Fatalf("online node OfflineSince = %v, want nil", on.OfflineSince)
	}
}
