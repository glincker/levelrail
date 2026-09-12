package store

import (
	"context"
	"testing"
	"time"
)

func TestDomainTLSCert_SetGetClear_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if _, found, err := db.GetDomainTLSCert(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false before any cert is uploaded")
	}

	uploaded := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := db.SetDomainTLSCert(ctx, "app.example.com", uploaded, expires); err != nil {
		t.Fatalf("SetDomainTLSCert() error = %v", err)
	}

	got, found, err := db.GetDomainTLSCert(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	}
	if !found || !got.UploadedAt.Equal(uploaded) || !got.ExpiresAt.Equal(expires) {
		t.Errorf("GetDomainTLSCert() = (%+v, %v), want UploadedAt=%v ExpiresAt=%v found=true", got, found, uploaded, expires)
	}

	// Setting again upserts rather than erroring or duplicating the row.
	expires2 := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := db.SetDomainTLSCert(ctx, "app.example.com", uploaded, expires2); err != nil {
		t.Fatalf("SetDomainTLSCert() (update) error = %v", err)
	}
	got, found, err = db.GetDomainTLSCert(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	}
	if !found || !got.ExpiresAt.Equal(expires2) {
		t.Errorf("GetDomainTLSCert() after update = (%+v, %v), want ExpiresAt=%v", got, found, expires2)
	}

	if err := db.DeleteDomainTLSCert(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainTLSCert() error = %v", err)
	}
	if _, found, err := db.GetDomainTLSCert(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after delete")
	}

	// Idempotent: deleting an already-cleared domain is not an error.
	if err := db.DeleteDomainTLSCert(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainTLSCert() (already cleared) error = %v", err)
	}
}

func TestDomainTLSCert_CascadesWhenDomainRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	now := time.Now().UTC()
	if err := db.SetDomainTLSCert(ctx, "app.example.com", now, now.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("SetDomainTLSCert() error = %v", err)
	}

	// Redeploying web with no domains removes its service_domains row,
	// which must cascade to drop the BYO cert row for it.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domain) error = %v", err)
	}

	if _, found, err := db.GetDomainTLSCert(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainTLSCert() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false: BYO cert should cascade-delete with its domain")
	}
}

func TestListDomainTLSCerts_OrderedByDomain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"zeta.example.com", "alpha.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	now := time.Now().UTC()
	if err := db.SetDomainTLSCert(ctx, "zeta.example.com", now, now.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("SetDomainTLSCert(zeta) error = %v", err)
	}
	if err := db.SetDomainTLSCert(ctx, "alpha.example.com", now, now.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("SetDomainTLSCert(alpha) error = %v", err)
	}

	got, err := db.ListDomainTLSCerts(ctx)
	if err != nil {
		t.Fatalf("ListDomainTLSCerts() error = %v", err)
	}
	if len(got) != 2 || got[0].Domain != "alpha.example.com" || got[1].Domain != "zeta.example.com" {
		t.Errorf("ListDomainTLSCerts() = %+v, want alpha then zeta", got)
	}
}

func TestDomainTLSCertSecretsKey_Stable(t *testing.T) {
	if got := DomainTLSCertSecretsKey("app.example.com"); got != DomainTLSCertSecretsKey("app.example.com") {
		t.Errorf("DomainTLSCertSecretsKey() is not stable across calls: %q vs %q", got, DomainTLSCertSecretsKey("app.example.com"))
	}
	if DomainTLSCertSecretsKey("a.example.com") == DomainTLSCertSecretsKey("b.example.com") {
		t.Errorf("DomainTLSCertSecretsKey() collides across distinct domains")
	}
}
