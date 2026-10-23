package nodehealth

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Agent link severed: the node is marked offline exactly once however many
// passes run, its placements are never touched, and a reconnect with a fresh
// heartbeat reports healthy without any further write.
func TestChaosPack_AgentSevered_NodeMarkedOfflineOnceAndPlacementsKept(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	es := &expiringStore{
		fakeStore: fakeStore{node: &store.Node{
			ID: "node-1", Status: store.NodeStatusOnline, LastSeenAt: ptrTime(now.Add(-5 * time.Minute)),
		}},
		services: []store.DesiredService{{Name: "web"}},
	}
	c := New("node-1", es, 45*time.Second, WithClock(clock), WithPendingExpiry(time.Hour))

	for i := 0; i < 3; i++ {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d error = %v", i, err)
		}
		if cond := res.Conditions[0]; cond.Status != reconcile.ConditionFalse || cond.Reason != "HeartbeatStale" {
			t.Fatalf("pass %d condition = %+v, want False/HeartbeatStale", i, cond)
		}
		if i == 0 {
			// The store applies the transition the controller asked for.
			es.node.Status = es.lastStatus
		}
	}
	if es.updateStatusCall != 1 {
		t.Fatalf("UpdateNodeStatus calls = %d, want 1: later passes must be no-ops", es.updateStatusCall)
	}
	if es.lastStatus != store.NodeStatusOffline {
		t.Fatalf("status = %q, want offline", es.lastStatus)
	}
	if es.deleted {
		t.Fatal("an unreachable node with placements was deleted")
	}

	es.node.Status = store.NodeStatusOnline
	es.node.LastSeenAt = ptrTime(now.Add(-2 * time.Second))
	calls := es.updateStatusCall
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("pass after reconnect error = %v", err)
	}
	if cond := res.Conditions[0]; cond.Status != reconcile.ConditionTrue || cond.Reason != "HeartbeatRecent" {
		t.Fatalf("condition after reconnect = %+v, want True/HeartbeatRecent", cond)
	}
	if es.updateStatusCall != calls {
		t.Fatal("a healthy node was written to on a reconcile pass")
	}
}
