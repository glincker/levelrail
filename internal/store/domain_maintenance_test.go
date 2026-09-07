package store

import (
	"context"
	"testing"
)

func TestDomainMaintenance_SetGetClear_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if enabled, err := db.GetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainMaintenance() error = %v", err)
	} else if enabled {
		t.Errorf("enabled = true, want false before maintenance mode is set")
	}

	if err := db.SetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("SetDomainMaintenance() error = %v", err)
	}

	if enabled, err := db.GetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainMaintenance() error = %v", err)
	} else if !enabled {
		t.Errorf("enabled = false, want true after SetDomainMaintenance")
	}

	// Setting again is a no-op, not an error.
	if err := db.SetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("SetDomainMaintenance() (repeat) error = %v", err)
	}

	if err := db.DeleteDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainMaintenance() error = %v", err)
	}
	if enabled, err := db.GetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainMaintenance() error = %v", err)
	} else if enabled {
		t.Errorf("enabled = true, want false after delete")
	}

	// Idempotent: deleting an already-cleared domain is not an error.
	if err := db.DeleteDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainMaintenance() (already cleared) error = %v", err)
	}
}

func TestDomainMaintenance_CascadesWhenDomainRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("SetDomainMaintenance() error = %v", err)
	}

	// Redeploying web with no domains removes its service_domains row,
	// which must cascade to drop the maintenance row for it.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domain) error = %v", err)
	}

	if enabled, err := db.GetDomainMaintenance(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainMaintenance() error = %v", err)
	} else if enabled {
		t.Errorf("enabled = true, want false: maintenance mode should cascade-delete with its domain")
	}
}

func TestListDomainMaintenance_OrderedByDomain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"zeta.example.com", "alpha.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainMaintenance(ctx, "zeta.example.com"); err != nil {
		t.Fatalf("SetDomainMaintenance(zeta) error = %v", err)
	}
	if err := db.SetDomainMaintenance(ctx, "alpha.example.com"); err != nil {
		t.Fatalf("SetDomainMaintenance(alpha) error = %v", err)
	}

	got, err := db.ListDomainMaintenance(ctx)
	if err != nil {
		t.Fatalf("ListDomainMaintenance() error = %v", err)
	}
	if len(got) != 2 || got[0] != "alpha.example.com" || got[1] != "zeta.example.com" {
		t.Errorf("ListDomainMaintenance() = %+v, want alpha then zeta", got)
	}
}
