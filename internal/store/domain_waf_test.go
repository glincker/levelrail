package store

import (
	"context"
	"testing"
)

func TestDomainWAF_SetGetClear_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if _, found, err := db.GetDomainWAF(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false before any WAF settings are set")
	}

	if err := db.SetDomainWAF(ctx, "app.example.com", true, DomainWAFModeBlock, 10, 20); err != nil {
		t.Fatalf("SetDomainWAF() error = %v", err)
	}

	got, found, err := db.GetDomainWAF(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	}
	if !found {
		t.Fatal("found = false, want true after SetDomainWAF")
	}
	want := DomainWAF{Domain: "app.example.com", WAFEnabled: true, WAFMode: DomainWAFModeBlock, RateLimitRPS: 10, RateLimitBurst: 20}
	if got != want {
		t.Errorf("GetDomainWAF() = %+v, want %+v", got, want)
	}

	// Setting again (upsert) overwrites every field, not just some.
	if err := db.SetDomainWAF(ctx, "app.example.com", false, DomainWAFModeDetect, 0, 0); err != nil {
		t.Fatalf("SetDomainWAF() (repeat) error = %v", err)
	}
	got, found, err = db.GetDomainWAF(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	}
	if !found {
		t.Fatal("found = false, want true after second SetDomainWAF")
	}
	want = DomainWAF{Domain: "app.example.com", WAFEnabled: false, WAFMode: DomainWAFModeDetect, RateLimitRPS: 0, RateLimitBurst: 0}
	if got != want {
		t.Errorf("GetDomainWAF() (after update) = %+v, want %+v", got, want)
	}

	if err := db.DeleteDomainWAF(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainWAF() error = %v", err)
	}
	if _, found, err := db.GetDomainWAF(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after delete")
	}

	// Idempotent: deleting an already-cleared domain is not an error.
	if err := db.DeleteDomainWAF(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainWAF() (already cleared) error = %v", err)
	}
}

func TestDomainWAF_CascadesWhenDomainRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainWAF(ctx, "app.example.com", true, DomainWAFModeBlock, 5, 5); err != nil {
		t.Fatalf("SetDomainWAF() error = %v", err)
	}

	// Redeploying web with no domains removes its service_domains row,
	// which must cascade to drop the WAF row for it.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domain) error = %v", err)
	}

	if _, found, err := db.GetDomainWAF(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false: WAF config should cascade-delete with its domain")
	}
}

func TestListDomainWAF_OrderedByDomain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"zeta.example.com", "alpha.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainWAF(ctx, "zeta.example.com", true, DomainWAFModeDetect, 0, 0); err != nil {
		t.Fatalf("SetDomainWAF(zeta) error = %v", err)
	}
	if err := db.SetDomainWAF(ctx, "alpha.example.com", true, DomainWAFModeDetect, 0, 0); err != nil {
		t.Fatalf("SetDomainWAF(alpha) error = %v", err)
	}

	got, err := db.ListDomainWAF(ctx)
	if err != nil {
		t.Fatalf("ListDomainWAF() error = %v", err)
	}
	if len(got) != 2 || got[0].Domain != "alpha.example.com" || got[1].Domain != "zeta.example.com" {
		t.Errorf("ListDomainWAF() = %+v, want alpha then zeta", got)
	}
}
