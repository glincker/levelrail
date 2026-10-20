package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func edgeRule(id string) EdgeFirewallRule {
	return EdgeFirewallRule{ID: id, Name: id, Enabled: true, Action: "deny", Conditions: `[{"field":"path","op":"equals","value":"/x"}]`, CreatedAt: "t", UpdatedAt: "t"}
}

func TestEdgeFirewallRules_CRUDAndOrder(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if _, err := db.CreateEdgeFirewallRule(ctx, edgeRule(id)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListEdgeFirewallRules(ctx)
	if err != nil || len(got) != 3 || got[0].ID != "a" || got[2].Position != 3 {
		t.Fatalf("list = %+v %v", got, err)
	}
	if err := db.ReorderEdgeFirewallRules(ctx, []string{"c", "a", "b"}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.ListEdgeFirewallRules(ctx)
	if ids := got[0].ID + got[1].ID + got[2].ID; ids != "cab" {
		t.Fatalf("order = %s", ids)
	}
	if err := db.ReorderEdgeFirewallRules(ctx, []string{"a"}); err == nil {
		t.Fatal("partial reorder should fail")
	}
	if err := db.ReorderEdgeFirewallRules(ctx, []string{"a", "b", "nope"}); !errors.Is(err, ErrEdgeFirewallRuleNotFound) {
		t.Fatalf("unknown id = %v", err)
	}
	r := edgeRule("a")
	r.Name, r.Enabled, r.UpdatedAt = "renamed", false, "t2"
	if err := db.UpdateEdgeFirewallRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	cur, err := db.GetEdgeFirewallRule(ctx, "a")
	if err != nil || cur.Name != "renamed" || cur.Enabled || cur.CreatedAt != "t" || cur.Position != 2 {
		t.Fatalf("after update = %+v %v", cur, err)
	}
	if err := db.UpdateEdgeFirewallRule(ctx, edgeRule("zz")); !errors.Is(err, ErrEdgeFirewallRuleNotFound) {
		t.Fatalf("update missing = %v", err)
	}
	if err := db.DeleteEdgeFirewallRule(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountEdgeFirewallRules(ctx); n != 2 {
		t.Fatalf("count = %d", n)
	}
	bad := edgeRule("x")
	bad.Action = "explode"
	if _, err := db.CreateEdgeFirewallRule(ctx, bad); err == nil {
		t.Fatal("unknown action should violate the check constraint")
	}
}

func TestEdgeFirewallSettings_SeededAndRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s, err := db.GetEdgeFirewallSettings(ctx)
	if err != nil || !strings.Contains(s, `"bans_enabled":true`) {
		t.Fatalf("seed = %q %v", s, err)
	}
	if err := db.SetEdgeFirewallSettings(ctx, `{"bot_mode":"block"}`); err != nil {
		t.Fatal(err)
	}
	if s, _ := db.GetEdgeFirewallSettings(ctx); s != `{"bot_mode":"block"}` {
		t.Fatalf("round trip = %q", s)
	}
}
