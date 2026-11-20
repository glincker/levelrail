package nodehealth

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// statusApplyingStore writes status changes back to the shared node like a real store.
type statusApplyingStore struct{ fakeStore }

func (s *statusApplyingStore) UpdateNodeStatus(ctx context.Context, id string, st store.NodeStatus) error {
	if err := s.fakeStore.UpdateNodeStatus(ctx, id, st); err != nil {
		return err
	}
	s.node.Status = st
	return nil
}

// The connect path marks the node online while its first last_seen write is still buffered.
func TestController_RecoversAfterEnrollmentRace(t *testing.T) {
	cur := time.Date(2026, 11, 20, 8, 26, 46, 0, time.UTC)
	node := &store.Node{ID: "node-1", Status: store.NodeStatusOnline, UpdatedAt: cur.Add(-2 * time.Second)}
	fs := &statusApplyingStore{fakeStore: fakeStore{node: node}}
	c := New("node-1", fs, 45*time.Second, WithClock(func() time.Time { return cur }))

	res, err := c.Reconcile(context.Background())
	if err != nil || node.Status != store.NodeStatusOnline {
		t.Fatalf("pass before first heartbeat: err=%v status=%s, want online", err, node.Status)
	}
	if res.Conditions[0].Reason != "AwaitingHeartbeat" {
		t.Fatalf("reason = %q", res.Conditions[0].Reason)
	}

	// The latched case from the field: marked offline anyway, then heartbeats resume.
	node.Status, node.UpdatedAt = store.NodeStatusOffline, cur
	cur = cur.Add(15 * time.Second)
	node.LastSeenAt = ptrTime(cur)
	res, err = c.Reconcile(context.Background())
	if err != nil || node.Status != store.NodeStatusOnline || res.Conditions[0].Status != reconcile.ConditionTrue {
		t.Fatalf("after heartbeat: err=%v status=%s, want online", err, node.Status)
	}
}

func TestController_GracefulDisconnectStaysOffline(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	node := &store.Node{ID: "node-1", Status: store.NodeStatusOffline, LastSeenAt: ptrTime(now.Add(-5 * time.Second)), UpdatedAt: now.Add(-3 * time.Second)}
	fs := &statusApplyingStore{fakeStore: fakeStore{node: node}}
	c := New("node-1", fs, 45*time.Second, WithClock(func() time.Time { return now }))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if node.Status != store.NodeStatusOffline || fs.updateStatusCall != 0 {
		t.Fatalf("status=%s calls=%d, want untouched offline", node.Status, fs.updateStatusCall)
	}
}

func TestController_DeadNodePastGraceGoesOffline(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	node := &store.Node{ID: "node-1", Status: store.NodeStatusOnline, UpdatedAt: now.Add(-2 * time.Minute)}
	fs := &statusApplyingStore{fakeStore: fakeStore{node: node}}
	c := New("node-1", fs, 45*time.Second, WithClock(func() time.Time { return now }))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if node.Status != store.NodeStatusOffline {
		t.Fatalf("status=%s, want offline", node.Status)
	}
}
