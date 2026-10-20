package store

import (
	"context"
	"testing"
)

func TestDomainRedirect_SetGetClear_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if _, found, err := db.GetDomainRedirect(ctx, "www.example.com"); err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false before a redirect is set")
	}

	if err := db.SetDomainRedirect(ctx, "www.example.com", "https://example.com", DomainRedirectPermanent); err != nil {
		t.Fatalf("SetDomainRedirect() error = %v", err)
	}

	got, found, err := db.GetDomainRedirect(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	}
	if !found || got.TargetURL != "https://example.com" || got.StatusCode != DomainRedirectPermanent {
		t.Errorf("GetDomainRedirect() = %+v, found=%v, want target_url=https://example.com status_code=301", got, found)
	}

	// Setting again (a different target/status) overwrites, not errors.
	if err := db.SetDomainRedirect(ctx, "www.example.com", "https://newapp.example.com/promo", DomainRedirectTemporary); err != nil {
		t.Fatalf("SetDomainRedirect() (overwrite) error = %v", err)
	}
	got, found, err = db.GetDomainRedirect(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	}
	if !found || got.TargetURL != "https://newapp.example.com/promo" || got.StatusCode != DomainRedirectTemporary {
		t.Errorf("GetDomainRedirect() = %+v, want the overwritten values", got)
	}

	if err := db.DeleteDomainRedirect(ctx, "www.example.com"); err != nil {
		t.Fatalf("DeleteDomainRedirect() error = %v", err)
	}
	if _, found, err := db.GetDomainRedirect(ctx, "www.example.com"); err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after delete")
	}

	// Idempotent: deleting an already-cleared domain is not an error.
	if err := db.DeleteDomainRedirect(ctx, "www.example.com"); err != nil {
		t.Fatalf("DeleteDomainRedirect() (already cleared) error = %v", err)
	}
}

func TestDomainRedirect_CascadesWhenDomainRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainRedirect(ctx, "www.example.com", "https://example.com", DomainRedirectPermanent); err != nil {
		t.Fatalf("SetDomainRedirect() error = %v", err)
	}

	// Redeploying web with no domains removes its service_domains row,
	// which must cascade to drop the redirect row for it.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domain) error = %v", err)
	}

	if _, found, err := db.GetDomainRedirect(ctx, "www.example.com"); err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false: a redirect should cascade-delete with its domain")
	}
}

func TestListDomainRedirects_OrderedByDomain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"zeta.example.com", "alpha.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainRedirect(ctx, "zeta.example.com", "https://example.com", DomainRedirectPermanent); err != nil {
		t.Fatalf("SetDomainRedirect(zeta) error = %v", err)
	}
	if err := db.SetDomainRedirect(ctx, "alpha.example.com", "https://example.com", DomainRedirectPermanent); err != nil {
		t.Fatalf("SetDomainRedirect(alpha) error = %v", err)
	}

	got, err := db.ListDomainRedirects(ctx)
	if err != nil {
		t.Fatalf("ListDomainRedirects() error = %v", err)
	}
	if len(got) != 2 || got[0].Domain != "alpha.example.com" || got[1].Domain != "zeta.example.com" {
		t.Errorf("ListDomainRedirects() = %+v, want alpha then zeta", got)
	}
}

func TestGetDomainRedirect_DefaultsStatusCodeToPermanent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, found, err := db.GetDomainRedirect(ctx, "unconfigured.example.com")
	if err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	}
	if found {
		t.Errorf("found = true, want false for an unconfigured domain")
	}
	if got.StatusCode != DomainRedirectPermanent {
		t.Errorf("StatusCode = %d, want %d as the zero-value default", got.StatusCode, DomainRedirectPermanent)
	}
}
