package store

import (
	"context"
	"errors"
	"testing"
)

func TestLBUpstreamAdminState_RoundTripAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SetLBUpstreamAdminState(ctx, "ghost", 0, "disabled"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Set on missing service error = %v, want ErrServiceNotFound", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct {
		replica int
		state   string
	}{{0, "draining"}, {1, "disabled"}, {0, "disabled"}} {
		if err := db.SetLBUpstreamAdminState(ctx, "web", s.replica, s.state); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListLBUpstreamAdminStates(ctx)
	if err != nil || got["web"][0] != "disabled" || got["web"][1] != "disabled" {
		t.Fatalf("List = %v err %v", got, err)
	}

	if err := db.SetLBUpstreamAdminState(ctx, "web", 0, "active"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.ListLBUpstreamAdminStates(ctx)
	if _, ok := got["web"][0]; ok || got["web"][1] != "disabled" {
		t.Fatalf("active must clear only replica 0, got %v", got)
	}

	if err := db.SetLBUpstreamAdminState(ctx, "web", 2, "bogus"); err == nil {
		t.Fatal("invalid state must be rejected by the table check")
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ListLBUpstreamAdminStates(ctx); len(got) != 0 {
		t.Fatalf("rows must cascade with the service, got %v", got)
	}
}
