package nodehealth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written fake, not a mocking framework, the same
// pattern database.fakeStore and application.fakeStore already
// establish for internal/store in this codebase.
type fakeStore struct {
	node *store.Node
	// getErr, when non-nil and not store.ErrNodeNotFound, simulates a
	// genuine store failure on GetNode.
	getErr error
	// updateStatusErr simulates UpdateNodeStatus failing, the "half
	// succeeded" case CLAUDE.md 7 requires a test for: the controller
	// has already decided the node is stale (correctly) but can't
	// persist that decision.
	updateStatusErr  error
	updateStatusCall int
	lastStatus       store.NodeStatus
}

func (f *fakeStore) GetNode(_ context.Context, _ string) (*store.Node, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.node, nil
}

func (f *fakeStore) UpdateNodeStatus(_ context.Context, _ string, status store.NodeStatus) error {
	f.updateStatusCall++
	f.lastStatus = status
	if f.updateStatusErr != nil {
		return f.updateStatusErr
	}
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestController_Reconcile(t *testing.T) {
	fixedNow := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedNow }
	const timeout = 45 * time.Second

	tests := []struct {
		name string
		node *store.Node

		wantConditionStatus reconcile.ConditionStatus
		wantReason          string
		wantErr             bool
		wantStatusUpdated   bool
		wantNewStatus       store.NodeStatus
	}{
		{
			name:                "pending node never connected",
			node:                &store.Node{ID: "node-1", Status: store.NodeStatusPending},
			wantConditionStatus: reconcile.ConditionUnknown,
			wantReason:          "NeverConnected",
		},
		{
			name: "online node with recent heartbeat stays online",
			node: &store.Node{
				ID: "node-1", Status: store.NodeStatusOnline,
				LastSeenAt: ptrTime(fixedNow.Add(-10 * time.Second)),
			},
			wantConditionStatus: reconcile.ConditionTrue,
			wantReason:          "HeartbeatRecent",
		},
		{
			name: "online node with stale heartbeat flips to offline",
			node: &store.Node{
				ID: "node-1", Status: store.NodeStatusOnline,
				LastSeenAt: ptrTime(fixedNow.Add(-90 * time.Second)),
			},
			wantConditionStatus: reconcile.ConditionFalse,
			wantReason:          "HeartbeatStale",
			wantStatusUpdated:   true,
			wantNewStatus:       store.NodeStatusOffline,
		},
		{
			name: "already offline node with stale heartbeat is a no-op (idempotent)",
			node: &store.Node{
				ID: "node-1", Status: store.NodeStatusOffline,
				LastSeenAt: ptrTime(fixedNow.Add(-90 * time.Second)),
			},
			wantConditionStatus: reconcile.ConditionFalse,
			wantReason:          "HeartbeatStale",
			wantStatusUpdated:   false,
		},
		{
			name:                "online node with no last_seen_at at all is stale",
			node:                &store.Node{ID: "node-1", Status: store.NodeStatusOnline, LastSeenAt: nil},
			wantConditionStatus: reconcile.ConditionFalse,
			wantReason:          "HeartbeatStale",
			wantStatusUpdated:   true,
			wantNewStatus:       store.NodeStatusOffline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeStore{node: tt.node}
			c := New("node-1", fs, timeout, WithClock(clock))

			result, err := c.Reconcile(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(result.Conditions) != 1 {
				t.Fatalf("Reconcile() Conditions = %+v, want exactly 1", result.Conditions)
			}
			cond := result.Conditions[0]
			if cond.Type != ConditionType {
				t.Errorf("Condition.Type = %q, want %q", cond.Type, ConditionType)
			}
			if cond.Status != tt.wantConditionStatus {
				t.Errorf("Condition.Status = %q, want %q", cond.Status, tt.wantConditionStatus)
			}
			if cond.Reason != tt.wantReason {
				t.Errorf("Condition.Reason = %q, want %q", cond.Reason, tt.wantReason)
			}
			if tt.wantStatusUpdated && fs.updateStatusCall != 1 {
				t.Errorf("UpdateNodeStatus call count = %d, want 1", fs.updateStatusCall)
			}
			if !tt.wantStatusUpdated && fs.updateStatusCall != 0 {
				t.Errorf("UpdateNodeStatus call count = %d, want 0", fs.updateStatusCall)
			}
			if tt.wantStatusUpdated && fs.lastStatus != tt.wantNewStatus {
				t.Errorf("UpdateNodeStatus status = %q, want %q", fs.lastStatus, tt.wantNewStatus)
			}
		})
	}
}

func TestController_Reconcile_NodeDeleted(t *testing.T) {
	fs := &fakeStore{getErr: store.ErrNodeNotFound}
	c := New("node-1", fs, time.Minute)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (a deleted node is not a failure)", err)
	}
	if len(result.Conditions) != 1 || result.Conditions[0].Status != reconcile.ConditionUnknown {
		t.Fatalf("Reconcile() Conditions = %+v, want one Unknown condition", result.Conditions)
	}
}

func TestController_Reconcile_StoreError(t *testing.T) {
	fs := &fakeStore{getErr: errors.New("db unavailable")}
	c := New("node-1", fs, time.Minute)

	_, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a wrapped store error")
	}
}

// TestController_Reconcile_HalfSucceeded is CLAUDE.md 7's required test
// for "the case where the operation half-succeeded": the controller
// correctly determines the node's heartbeat is stale, but persisting
// that (UpdateNodeStatus) fails. Reconcile must surface the failure
// (non-nil error, an Unknown condition rather than silently claiming
// False/HeartbeatStale succeeded), and because this controller is
// level-triggered and never caches anything between calls, a second
// Reconcile against the same still-stale node must retry the exact same
// UpdateNodeStatus call and succeed once the store recovers, without
// needing any special "resume" logic.
func TestController_Reconcile_HalfSucceeded(t *testing.T) {
	fixedNow := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedNow }
	node := &store.Node{
		ID: "node-1", Status: store.NodeStatusOnline,
		LastSeenAt: ptrTime(fixedNow.Add(-90 * time.Second)),
	}
	fs := &fakeStore{node: node, updateStatusErr: errors.New("write conflict")}
	c := New("node-1", fs, 45*time.Second, WithClock(clock))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want a wrapped UpdateNodeStatus error")
	}
	if len(result.Conditions) != 1 || result.Conditions[0].Status != reconcile.ConditionUnknown || result.Conditions[0].Reason != "MarkOfflineFailed" {
		t.Fatalf("Reconcile() Conditions = %+v, want one Unknown/MarkOfflineFailed condition", result.Conditions)
	}
	if fs.updateStatusCall != 1 {
		t.Fatalf("UpdateNodeStatus call count = %d, want 1", fs.updateStatusCall)
	}

	// Retry: the store recovers, node observed state is unchanged (still
	// Online, still stale, exactly as it would be on the engine's very
	// next resync tick). This must re-derive the same conclusion and
	// this time actually converge, with no memory of the previous
	// failed attempt.
	fs.updateStatusErr = nil
	result, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("retry Reconcile() error = %v, want nil", err)
	}
	if len(result.Conditions) != 1 || result.Conditions[0].Status != reconcile.ConditionFalse || result.Conditions[0].Reason != "HeartbeatStale" {
		t.Fatalf("retry Reconcile() Conditions = %+v, want one False/HeartbeatStale condition", result.Conditions)
	}
	if fs.updateStatusCall != 2 {
		t.Fatalf("UpdateNodeStatus call count after retry = %d, want 2", fs.updateStatusCall)
	}
	if fs.lastStatus != store.NodeStatusOffline {
		t.Errorf("lastStatus = %q, want %q", fs.lastStatus, store.NodeStatusOffline)
	}
}

func TestController_Name(t *testing.T) {
	c := New("node-abc", &fakeStore{}, time.Minute)
	if got, want := c.Name(), "node-health/node-abc"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}
