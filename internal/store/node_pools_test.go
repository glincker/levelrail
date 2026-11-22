package store

import (
	"context"
	"errors"
	"testing"
)

func TestNodePoolsAndWarm(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetNodePool(ctx, "gpu", map[string]string{"role": "gpu"}); err != nil {
		t.Fatalf("SetNodePool() error = %v", err)
	}
	if err := db.SetAppPool(ctx, "web", "nope"); !errors.Is(err, ErrPoolNotFound) {
		t.Fatalf("SetAppPool(unknown) error = %v, want ErrPoolNotFound", err)
	}
	if err := db.SetAppPool(ctx, "web", "gpu"); err != nil {
		t.Fatalf("SetAppPool() error = %v", err)
	}
	if got, _ := db.GetAppPool(ctx, "web"); got != "gpu" {
		t.Fatalf("GetAppPool() = %q, want gpu", got)
	}
	if err := db.DeleteNodePool(ctx, "gpu"); !errors.Is(err, ErrPoolInUse) {
		t.Fatalf("DeleteNodePool(in use) error = %v, want ErrPoolInUse", err)
	}
	if err := db.SetAppPool(ctx, "web", ""); err != nil {
		t.Fatalf("clear pool error = %v", err)
	}
	if err := db.DeleteNodePool(ctx, "gpu"); err != nil {
		t.Fatalf("DeleteNodePool() error = %v", err)
	}

	want := WarmTargets{Nodes: []string{"n1", "n2"}, Labels: map[string]string{"zone": "a"}}
	if err := db.SetAppWarm(ctx, "web", want); err != nil {
		t.Fatalf("SetAppWarm() error = %v", err)
	}
	got, err := db.GetAppWarm(ctx, "web")
	if err != nil || len(got.Nodes) != 2 || got.Labels["zone"] != "a" {
		t.Fatalf("GetAppWarm() = %+v, %v", got, err)
	}
	if err := db.SetAppWarm(ctx, "web", WarmTargets{}); err != nil {
		t.Fatalf("clear warm error = %v", err)
	}
	if got, _ := db.GetAppWarm(ctx, "web"); len(got.Nodes) != 0 || len(got.Labels) != 0 {
		t.Fatalf("after clear = %+v", got)
	}
}
