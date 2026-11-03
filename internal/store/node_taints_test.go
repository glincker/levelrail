package store

import (
	"context"
	"errors"
	"testing"
)

func TestNodeTaintsRoundTripReplaceCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveNode(ctx, Node{ID: "n1", Name: "one", Status: NodeStatusOnline}); err != nil {
		t.Fatal(err)
	}
	first := []NodeTaint{{Key: "dedicated", Value: "gpu", Effect: "NoSchedule"}, {Key: "spot", Effect: "PreferNoSchedule"}}
	if err := db.SetNodeTaints(ctx, "n1", first); err != nil {
		t.Fatalf("SetNodeTaints() error = %v", err)
	}
	got, err := db.GetNodeTaints(ctx, "n1")
	if err != nil || len(got) != 2 || got[0] != first[0] || got[1] != first[1] {
		t.Fatalf("GetNodeTaints() = %v, %v", got, err)
	}
	if err := db.SetNodeTaints(ctx, "n1", []NodeTaint{{Key: "old", Effect: "NoExecute"}}); err != nil {
		t.Fatal(err)
	}
	all, err := db.ListNodeTaints(ctx)
	if err != nil || len(all["n1"]) != 1 || all["n1"][0].Key != "old" {
		t.Fatalf("ListNodeTaints() = %v, %v", all, err)
	}
	if err := db.SetNodeTaints(ctx, "n1", []NodeTaint{{Key: "bad", Effect: "Whenever"}}); err == nil {
		t.Fatal("invalid effect accepted")
	}
	if got, _ := db.GetNodeTaints(ctx, "n1"); len(got) != 1 {
		t.Fatalf("failed replace must keep the old set, got %v", got)
	}
	if err := db.SetNodeTaints(ctx, "ghost", nil); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("SetNodeTaints(ghost) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM nodes WHERE id = 'n1'`); err != nil {
		t.Fatal(err)
	}
	if all, _ := db.ListNodeTaints(ctx); len(all) != 0 {
		t.Fatalf("taints survived node delete: %v", all)
	}
}
