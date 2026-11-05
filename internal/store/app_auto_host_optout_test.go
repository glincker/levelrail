package store

import "testing"

func TestAppAutoHostOptOutRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	if off, err := db.GetAppAutoHostOptOut(ctx, "web"); err != nil || off {
		t.Fatalf("default = %v, %v, want not opted out", off, err)
	}
	if err := db.SetAppAutoHostOptOut(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppAutoHostOptOut(ctx, "web", true); err != nil {
		t.Fatalf("repeat set: %v", err)
	}
	m, err := db.ListAppAutoHostOptOuts(ctx)
	if err != nil || !m["web"] || len(m) != 1 {
		t.Fatalf("list = %v, %v", m, err)
	}
	if err := db.SetAppAutoHostOptOut(ctx, "web", false); err != nil {
		t.Fatal(err)
	}
	if off, _ := db.GetAppAutoHostOptOut(ctx, "web"); off {
		t.Fatal("opt-out not cleared")
	}
}
