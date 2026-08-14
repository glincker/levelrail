package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testNode(id, name string) Node {
	now := time.Now()
	return Node{
		ID:        id,
		Name:      name,
		Address:   "10.0.0.1:9443",
		Status:    NodeStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestSaveAndGetNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := testNode("node_1", "worker-1")
	if err := db.SaveNode(ctx, want); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}

	got, err := db.GetNode(ctx, "node_1")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name || got.Address != want.Address {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Status != NodeStatusPending {
		t.Errorf("got.Status = %q, want %q", got.Status, NodeStatusPending)
	}
	if got.JoinedAt != nil || got.LastSeenAt != nil {
		t.Errorf("got JoinedAt=%v LastSeenAt=%v, want both nil for a freshly saved node", got.JoinedAt, got.LastSeenAt)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetNode(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("GetNode() error = %v, want ErrNodeNotFound", err)
	}
}

func TestSaveNode_DuplicateName_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_1", "worker-1")); err != nil {
		t.Fatalf("first SaveNode() error = %v", err)
	}
	err := db.SaveNode(ctx, testNode("node_2", "worker-1")) // same name, different ID
	if !errors.Is(err, ErrNodeNameTaken) {
		t.Errorf("second SaveNode() error = %v, want ErrNodeNameTaken", err)
	}

	// The rejected save must not have clobbered the original row.
	got, err := db.GetNode(ctx, "node_1")
	if err != nil {
		t.Fatalf("GetNode(node_1) error = %v", err)
	}
	if got.Name != "worker-1" {
		t.Errorf("original node's name = %q, want unchanged worker-1", got.Name)
	}
}

func TestListNodes_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_b", "zebra")); err != nil {
		t.Fatalf("SaveNode(zebra) error = %v", err)
	}
	if err := db.SaveNode(ctx, testNode("node_a", "alpha")); err != nil {
		t.Fatalf("SaveNode(alpha) error = %v", err)
	}

	got, err := db.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zebra" {
		t.Fatalf("ListNodes() = %+v, want [alpha zebra] in that order", got)
	}
}

func TestListNodes_Empty(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListNodes() = %d nodes, want 0", len(got))
	}
}

func TestDeleteNode_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_1", "worker-1")); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	if err := db.DeleteNode(ctx, "node_1"); err != nil {
		t.Fatalf("first DeleteNode() error = %v", err)
	}
	if err := db.DeleteNode(ctx, "node_1"); err != nil {
		t.Errorf("second DeleteNode() (already gone) error = %v, want nil (idempotent)", err)
	}

	_, err := db.GetNode(ctx, "node_1")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("GetNode() after delete error = %v, want ErrNodeNotFound", err)
	}
}

func TestUpdateNodeStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_1", "worker-1")); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	if err := db.UpdateNodeStatus(ctx, "node_1", NodeStatusOnline); err != nil {
		t.Fatalf("UpdateNodeStatus() error = %v", err)
	}

	got, err := db.GetNode(ctx, "node_1")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if got.Status != NodeStatusOnline {
		t.Errorf("got.Status = %q, want %q", got.Status, NodeStatusOnline)
	}
}

func TestUpdateNodeStatus_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateNodeStatus(context.Background(), "nonexistent", NodeStatusOnline)
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("UpdateNodeStatus() error = %v, want ErrNodeNotFound", err)
	}
}

func TestSaveNode_WorkloadFlagsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	n := testNode("node_1", "worker-1")
	n.AcceptsAppWorkloads = false
	n.AcceptsBuildWorkloads = true
	if err := db.SaveNode(ctx, n); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}

	got, err := db.GetNode(ctx, "node_1")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if got.AcceptsAppWorkloads != false {
		t.Errorf("got.AcceptsAppWorkloads = %v, want false", got.AcceptsAppWorkloads)
	}
	if got.AcceptsBuildWorkloads != true {
		t.Errorf("got.AcceptsBuildWorkloads = %v, want true", got.AcceptsBuildWorkloads)
	}
}

func TestUpdateNodeWorkloads(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_1", "worker-1")); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	if err := db.UpdateNodeWorkloads(ctx, "node_1", false, true); err != nil {
		t.Fatalf("UpdateNodeWorkloads() error = %v", err)
	}

	got, err := db.GetNode(ctx, "node_1")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if got.AcceptsAppWorkloads != false || got.AcceptsBuildWorkloads != true {
		t.Errorf("got = %+v, want AcceptsAppWorkloads=false AcceptsBuildWorkloads=true", got)
	}
}

func TestUpdateNodeWorkloads_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateNodeWorkloads(context.Background(), "nonexistent", true, true)
	if !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("UpdateNodeWorkloads() error = %v, want ErrNodeNotFound", err)
	}
}

func TestTouchNodeLastSeen(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveNode(ctx, testNode("node_1", "worker-1")); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	before := time.Now().Add(-time.Second)
	if err := db.TouchNodeLastSeen(ctx, "node_1"); err != nil {
		t.Fatalf("TouchNodeLastSeen() error = %v", err)
	}

	got, err := db.GetNode(ctx, "node_1")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if got.LastSeenAt == nil {
		t.Fatal("got.LastSeenAt = nil, want a timestamp after TouchNodeLastSeen")
	}
	if got.LastSeenAt.Before(before) {
		t.Errorf("got.LastSeenAt = %v, want at or after %v", got.LastSeenAt, before)
	}
}
