package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCustomDashboards(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	mk := func(id, owner, scope, name string, def bool) CustomDashboard {
		return CustomDashboard{ID: id, Owner: owner, Scope: scope, ProjectID: "p1", IsDefault: def, Name: name, Body: `{}`, CreatedAt: now, UpdatedAt: now}
	}
	for _, d := range []CustomDashboard{
		mk("a", "user:1", "user", "mine", false),
		mk("b", "user:2", "user", "mine", false),
		mk("c", "user:2", "shared", "team", true),
	} {
		if err := db.CreateCustomDashboard(ctx, d); err != nil {
			t.Fatalf("create %s: %v", d.ID, err)
		}
	}
	if err := db.CreateCustomDashboard(ctx, mk("d", "user:1", "user", "mine", false)); !errors.Is(err, ErrDashboardNameTaken) {
		t.Fatalf("duplicate = %v", err)
	}
	if err := db.CreateCustomDashboard(ctx, mk("e", "user:1", "shared", "team", false)); !errors.Is(err, ErrDashboardNameTaken) {
		t.Fatalf("shared duplicate = %v", err)
	}

	rows, err := db.ListCustomDashboards(ctx, "user:1", "")
	if err != nil || len(rows) != 2 || rows[0].ID != "c" {
		t.Fatalf("user:1 sees %+v %v", rows, err)
	}

	second := mk("f", "user:1", "shared", "ops", true)
	if err := db.CreateCustomDashboard(ctx, second); err != nil {
		t.Fatalf("second default: %v", err)
	}
	got, _ := db.GetCustomDashboard(ctx, "c")
	if got.IsDefault {
		t.Fatal("older default was not cleared")
	}

	if err := db.DeleteCustomDashboard(ctx, "f"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.GetCustomDashboard(ctx, "f"); !errors.Is(err, ErrDashboardNotFound) {
		t.Fatalf("get deleted = %v", err)
	}
}
