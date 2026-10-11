package store

import (
	"context"
	"errors"
	"testing"
)

func TestNodeMove_LockAndLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	m := NodeMove{ID: "mv1", Kind: NodeMoveKindApp, Resource: "web", FromNodeID: "a", ToNodeID: "b",
		Stateful: true, Volumes: []string{"data"}, Phase: "preflight", StartedAt: "2026-10-10T00:00:00Z"}
	if err := db.CreateNodeMove(ctx, m); err != nil {
		t.Fatalf("CreateNodeMove() error = %v", err)
	}
	second := m
	second.ID = "mv2"
	if err := db.CreateNodeMove(ctx, second); !errors.Is(err, ErrNodeMoveInProgress) {
		t.Fatalf("second CreateNodeMove() error = %v, want ErrNodeMoveInProgress", err)
	}

	got, err := db.GetNodeMove(ctx, "mv1")
	if err != nil {
		t.Fatalf("GetNodeMove() error = %v", err)
	}
	if !got.Stateful || len(got.Volumes) != 1 || got.Status != NodeMoveStatusRunning {
		t.Fatalf("got = %+v", got)
	}
	got.Status = NodeMoveStatusSucceeded
	got.Phase = "done"
	got.Steps = []NodeMoveStep{{Phase: "preflight", Outcome: "ok", At: "x"}}
	if err := db.SaveNodeMove(ctx, got); err != nil {
		t.Fatalf("SaveNodeMove() error = %v", err)
	}
	if err := db.CreateNodeMove(ctx, second); err != nil {
		t.Fatalf("CreateNodeMove() after finish error = %v, want lock released", err)
	}
	list, err := db.ListNodeMoves(ctx, NodeMoveFilter{Kind: NodeMoveKindApp, Resource: "web"})
	if err != nil || len(list) != 2 {
		t.Fatalf("ListNodeMoves() = %d, %v; want 2", len(list), err)
	}
	running, _ := db.ListNodeMoves(ctx, NodeMoveFilter{Status: NodeMoveStatusRunning})
	if len(running) != 1 || running[0].ID != "mv2" {
		t.Fatalf("running = %+v", running)
	}
	if err := db.SaveNodeMove(ctx, NodeMove{ID: "missing"}); !errors.Is(err, ErrNodeMoveNotFound) {
		t.Fatalf("SaveNodeMove(missing) error = %v", err)
	}
}

func TestNodeDrain_OnePerNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	d := NodeDrain{ID: "dr1", NodeID: "a", StartedAt: "2026-10-10T00:00:00Z"}
	if err := db.CreateNodeDrain(ctx, d); err != nil {
		t.Fatalf("CreateNodeDrain() error = %v", err)
	}
	if err := db.CreateNodeDrain(ctx, NodeDrain{ID: "dr2", NodeID: "a", StartedAt: "x"}); !errors.Is(err, ErrNodeMoveInProgress) {
		t.Fatalf("second drain error = %v", err)
	}
	d.Status = NodeMoveStatusFailed
	d.Blocked = []NodeDrainBlocked{{Kind: "model", Name: "m", Reason: "r"}}
	if err := db.SaveNodeDrain(ctx, d); err != nil {
		t.Fatalf("SaveNodeDrain() error = %v", err)
	}
	got, err := db.GetNodeDrain(ctx, "dr1")
	if err != nil || len(got.Blocked) != 1 || got.Status != NodeMoveStatusFailed {
		t.Fatalf("GetNodeDrain() = %+v, %v", got, err)
	}
	active, _ := db.ListNodeDrains(ctx, "", 0)
	if len(active) != 0 {
		t.Fatalf("active drains = %+v, want none", active)
	}
}

func TestNodeLabelsAndSelectors(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveNode(ctx, Node{ID: "n1", Name: "one", Status: NodeStatusOnline}); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	if err := db.SetNodeLabels(ctx, "n1", map[string]string{"disk": "ssd", "zone": "a"}); err != nil {
		t.Fatalf("SetNodeLabels() error = %v", err)
	}
	if err := db.SetNodeLabels(ctx, "n1", map[string]string{"disk": "hdd"}); err != nil {
		t.Fatalf("SetNodeLabels() replace error = %v", err)
	}
	got, _ := db.GetNodeLabels(ctx, "n1")
	if len(got) != 1 || got["disk"] != "hdd" {
		t.Fatalf("labels = %v, want replaced set", got)
	}
	if err := db.SetNodeLabels(ctx, "ghost", nil); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("SetNodeLabels(ghost) error = %v", err)
	}
	if err := db.SetPlacementSelector(ctx, NodeMoveKindApp, "web", map[string]string{"disk": "ssd"}); err != nil {
		t.Fatalf("SetPlacementSelector() error = %v", err)
	}
	sel, _ := db.GetPlacementSelector(ctx, NodeMoveKindApp, "web")
	if sel["disk"] != "ssd" {
		t.Fatalf("selector = %v", sel)
	}
}

func TestEnsureDatabaseMeshPort(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	p1, err := db.EnsureDatabaseMeshPort(ctx, "main", 21000, 21001)
	if err != nil || p1 != 21000 {
		t.Fatalf("first = %d, %v", p1, err)
	}
	again, _ := db.EnsureDatabaseMeshPort(ctx, "main", 21000, 21001)
	if again != p1 {
		t.Fatalf("not idempotent: %d != %d", again, p1)
	}
	p2, _ := db.EnsureDatabaseMeshPort(ctx, "cache", 21000, 21001)
	if p2 != 21001 {
		t.Fatalf("second db port = %d, want 21001", p2)
	}
	if _, err := db.EnsureDatabaseMeshPort(ctx, "third", 21000, 21001); !errors.Is(err, ErrMeshPortsExhausted) {
		t.Fatalf("exhausted error = %v", err)
	}
	if _, err := db.EnsureDatabaseMeshPort(ctx, "bad", 0, 1); err == nil {
		t.Fatal("invalid range accepted")
	}
}
