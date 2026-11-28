package store

import (
	"context"
	"slices"
	"testing"
)

func TestUpdateReplicaNodesBumpsFenceOnMove(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:1", Port: 80, Replicas: 2, NodeID: "n1"}); err != nil {
		t.Fatal(err)
	}
	before, _ := db.ServiceFenceToken(ctx, "web")

	if err := db.UpdateServiceReplicaNodes(ctx, "web", []string{"n1", "n2"}); err != nil {
		t.Fatal(err)
	}
	moved, _ := db.ServiceFenceToken(ctx, "web")
	if moved <= before {
		t.Fatalf("token %d after a move, want above %d", moved, before)
	}

	if err := db.UpdateServiceReplicaNodes(ctx, "web", []string{"n1", "n2"}); err != nil {
		t.Fatal(err)
	}
	if same, _ := db.ServiceFenceToken(ctx, "web"); same != moved {
		t.Fatalf("token changed to %d with an unchanged plan", same)
	}

	if err := db.UpdateServiceReplicaNodes(ctx, "web", nil); err != nil {
		t.Fatal(err)
	}
	cleared, _ := db.ServiceFenceToken(ctx, "web")
	if cleared <= moved {
		t.Fatalf("token %d after clearing the spread, want above %d", cleared, moved)
	}
	got, _ := db.GetDesiredService(ctx, "web")
	if !slices.Equal(got.ReplicaNodes, nil) {
		t.Fatalf("ReplicaNodes = %v, want none", got.ReplicaNodes)
	}
}
