package store

import (
	"testing"
	"time"
)

func TestStackDrift_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	s := newTestStack(t, db, "shop-drift")
	if _, ok, err := db.GetStackDrift(ctx, s.ID); ok || err != nil {
		t.Fatalf("before first check = %v, %v", ok, err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	in := StackDrift{StackID: s.ID, Status: StackDriftDrifted, Revision: 3, CheckedAt: at,
		Summary: []StackDriftItem{{Kind: "App", Name: "web", Action: "update", Fields: []string{"env.LOG_LEVEL"}}}}
	if err := db.SetStackDrift(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetStackDrift(ctx, s.ID)
	if err != nil || !ok || got.Status != StackDriftDrifted || got.Revision != 3 || !got.CheckedAt.Equal(at) || len(got.Summary) != 1 || got.Summary[0].Fields[0] != "env.LOG_LEVEL" {
		t.Fatalf("got = %+v, %v, %v", got, ok, err)
	}
	in.Status, in.Summary = StackDriftNone, nil
	if err := db.SetStackDrift(ctx, in); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListStackDrift(ctx)
	if err != nil || len(list) != 1 || list[0].Status != StackDriftNone || len(list[0].Summary) != 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.SetStackDrift(ctx, StackDrift{StackID: s.ID, Status: "weird", CheckedAt: at}); err == nil {
		t.Fatal("unknown status accepted")
	}
	if _, err := db.DeleteStack(ctx, s.ID, "forget"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetStackDrift(ctx, s.ID); ok {
		t.Fatal("drift row outlived its stack")
	}
}
