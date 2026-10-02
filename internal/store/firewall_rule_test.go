package store

import (
	"context"
	"errors"
	"testing"
)

func newTestFirewallRule() FirewallRule {
	return FirewallRule{
		ID:         "fwrule_test1",
		NodeID:     "",
		Port:       8443,
		Protocol:   "tcp",
		SourceCIDR: "10.0.0.0/24",
		Action:     FirewallRuleActionAllow,
		Label:      "vpn access",
		CreatedAt:  "2026-10-01T00:00:00Z",
	}
}

func TestSaveAndGetFirewallRule(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestFirewallRule()
	if err := db.SaveFirewallRule(ctx, want); err != nil {
		t.Fatalf("SaveFirewallRule() error = %v", err)
	}

	got, err := db.GetFirewallRule(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetFirewallRule() error = %v", err)
	}
	if got != want {
		t.Errorf("GetFirewallRule() = %+v, want %+v", got, want)
	}
}

func TestGetFirewallRule_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetFirewallRule(ctx, "missing")
	if !errors.Is(err, ErrFirewallRuleNotFound) {
		t.Fatalf("GetFirewallRule() error = %v, want ErrFirewallRuleNotFound", err)
	}
}

func TestListFirewallRules_OldestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := FirewallRule{ID: "fwrule_a", Port: 22, Protocol: "tcp", Action: FirewallRuleActionDeny, CreatedAt: "2026-10-01T00:00:00Z"}
	second := FirewallRule{ID: "fwrule_b", Port: 9000, Protocol: "tcp", Action: FirewallRuleActionAllow, CreatedAt: "2026-10-02T00:00:00Z"}
	if err := db.SaveFirewallRule(ctx, second); err != nil {
		t.Fatalf("SaveFirewallRule(second) error = %v", err)
	}
	if err := db.SaveFirewallRule(ctx, first); err != nil {
		t.Fatalf("SaveFirewallRule(first) error = %v", err)
	}

	got, err := db.ListFirewallRules(ctx)
	if err != nil {
		t.Fatalf("ListFirewallRules() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "fwrule_a" || got[1].ID != "fwrule_b" {
		t.Fatalf("ListFirewallRules() = %+v, want [fwrule_a, fwrule_b]", got)
	}
}

func TestDeleteFirewallRule(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestFirewallRule()
	if err := db.SaveFirewallRule(ctx, want); err != nil {
		t.Fatalf("SaveFirewallRule() error = %v", err)
	}
	if err := db.DeleteFirewallRule(ctx, want.ID); err != nil {
		t.Fatalf("DeleteFirewallRule() error = %v", err)
	}
	if _, err := db.GetFirewallRule(ctx, want.ID); !errors.Is(err, ErrFirewallRuleNotFound) {
		t.Fatalf("GetFirewallRule() after delete error = %v, want ErrFirewallRuleNotFound", err)
	}
}

func TestDeleteFirewallRule_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteFirewallRule(ctx, "missing")
	if !errors.Is(err, ErrFirewallRuleNotFound) {
		t.Fatalf("DeleteFirewallRule() error = %v, want ErrFirewallRuleNotFound", err)
	}
}
