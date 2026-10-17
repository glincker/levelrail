package store

import (
	"context"
	"testing"
	"time"
)

func TestPlacementDecisionRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, ok, err := db.GetPlacementDecision(ctx, "app", "web"); ok || err != nil {
		t.Fatalf("empty get = %v, %v", ok, err)
	}
	first := PlacementDecision{Kind: "app", Resource: "web", Trigger: "create", NodeID: "n1", DecidedAt: time.Unix(100, 0), Body: []byte(`{"a":1}`)}
	second := first
	second.NodeID, second.Trigger, second.Body = "n2", "move", []byte(`{"a":2}`)
	for _, d := range []PlacementDecision{first, second} {
		if err := db.SavePlacementDecision(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	got, ok, err := db.GetPlacementDecision(ctx, "app", "web")
	if err != nil || !ok || got.NodeID != "n2" || got.Trigger != "move" || string(got.Body) != `{"a":2}` {
		t.Fatalf("get = %+v, %v, %v", got, ok, err)
	}
}
