package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GLINCKER/levelrail/internal/placement"
)

func scoreNode(id string, cpu int64) placement.Candidate {
	return placement.Candidate{
		ID: id, Schedulable: true, Online: true,
		Capacity: placement.Capacity{
			CPUMilli: placement.Measured(4000), MemoryBytes: placement.Measured(8 << 30),
			DiskTotalBytes: placement.Measured(100 << 30), DiskFreeBytes: placement.Measured(90 << 30),
		},
		Requested: placement.Resources{CPUMilli: cpu},
	}
}

func pickNode(t *testing.T, rt *Router) string {
	t.Helper()
	cands := []placement.Candidate{scoreNode("empty", 0), scoreNode("full", 3500)}
	req := placement.Request{Needs: placement.Resources{CPUMilli: 400}, Policy: rt.currentPlacementPolicy(context.Background())}
	d, err := placement.Spread(cands, req)
	if err != nil {
		t.Fatal(err)
	}
	return d.NodeID
}

func TestPlacementScoreModeFlipsLive(t *testing.T) {
	rt, _, _ := newMultiNodeRouter(t)
	ctx := context.Background()
	if got := pickNode(t, rt); got != "empty" {
		t.Fatalf("default should spread to the emptiest node, got %s", got)
	}
	if _, err := rt.runtimeSettings.Set(ctx, KeyPlacementScoreMode, json.RawMessage(`"binpack"`), "test"); err != nil {
		t.Fatal(err)
	}
	if got := pickNode(t, rt); got != "full" {
		t.Fatalf("binpack should fill the fullest node, got %s", got)
	}
	if _, err := rt.runtimeSettings.Set(ctx, KeyPlacementScoreMode, json.RawMessage(`"spread"`), "test"); err != nil {
		t.Fatal(err)
	}
	if got := pickNode(t, rt); got != "empty" {
		t.Fatalf("flipping back should spread again, got %s", got)
	}
}

func TestPlacementScoreModeInvalidFallsBackToSpread(t *testing.T) {
	rt, _, _ := newMultiNodeRouter(t)
	if _, err := rt.runtimeSettings.Set(context.Background(), KeyPlacementScoreMode, json.RawMessage(`"random"`), "test"); err == nil {
		t.Fatal("an unknown mode must be rejected")
	}
	if got := pickNode(t, rt); got != "empty" {
		t.Fatalf("invalid value must fall back to spread, got %s", got)
	}
	rt.runtimeSettings = nil
	if p := rt.currentPlacementPolicy(context.Background()); p.ScoreMode != placement.ScoreSpread {
		t.Fatalf("no settings service should mean spread, got %q", p.ScoreMode)
	}
}
