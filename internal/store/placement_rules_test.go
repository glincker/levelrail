package store

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestPlacementRulesRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	tols := json.RawMessage(`[{"key":"dedicated","operator":"Exists","effect":"NoSchedule"}]`)
	aff := json.RawMessage(`{"node_required":[{"key":"disk","op":"In","values":["ssd"]}]}`)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:1", Tolerations: tols, Affinity: aff}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tolerations, tols) || !reflect.DeepEqual(got.Affinity, aff) {
		t.Fatalf("round trip = %+v %+v", got.Tolerations, got.Affinity)
	}
	if err := db.UpdateServicePlacementRules(ctx, "web", nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetDesiredService(ctx, "web")
	if got.Tolerations != nil || got.Affinity != nil {
		t.Fatalf("cleared rules = %+v %+v", got.Tolerations, got.Affinity)
	}
}
