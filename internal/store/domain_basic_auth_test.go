package store

import (
	"context"
	"testing"
)

func TestDomainBasicAuth_SetGetClear_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if _, found, err := db.GetDomainBasicAuth(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false before any basic auth is set")
	}

	if err := db.SetDomainBasicAuth(ctx, "app.example.com", "operator"); err != nil {
		t.Fatalf("SetDomainBasicAuth() error = %v", err)
	}

	got, found, err := db.GetDomainBasicAuth(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	}
	if !found || got.Username != "operator" {
		t.Errorf("GetDomainBasicAuth() = (%+v, %v), want Username=operator, found=true", got, found)
	}

	// Setting again upserts rather than erroring or duplicating the row.
	if err := db.SetDomainBasicAuth(ctx, "app.example.com", "operator2"); err != nil {
		t.Fatalf("SetDomainBasicAuth() (update) error = %v", err)
	}
	got, found, err = db.GetDomainBasicAuth(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	}
	if !found || got.Username != "operator2" {
		t.Errorf("GetDomainBasicAuth() after update = (%+v, %v), want Username=operator2, found=true", got, found)
	}

	if err := db.DeleteDomainBasicAuth(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainBasicAuth() error = %v", err)
	}
	if _, found, err := db.GetDomainBasicAuth(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after delete")
	}

	// Idempotent: deleting an already-cleared domain is not an error.
	if err := db.DeleteDomainBasicAuth(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainBasicAuth() (already cleared) error = %v", err)
	}
}

func TestDomainBasicAuth_CascadesWhenDomainRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainBasicAuth(ctx, "app.example.com", "operator"); err != nil {
		t.Fatalf("SetDomainBasicAuth() error = %v", err)
	}

	// Redeploying web with no domains removes its service_domains row,
	// which must cascade to drop the basic auth row protecting it.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domain) error = %v", err)
	}

	if _, found, err := db.GetDomainBasicAuth(ctx, "app.example.com"); err != nil {
		t.Fatalf("GetDomainBasicAuth() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false: basic auth should cascade-delete with its domain")
	}
}

func TestListDomainBasicAuth_OrderedByDomain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"zeta.example.com", "alpha.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainBasicAuth(ctx, "zeta.example.com", "z-user"); err != nil {
		t.Fatalf("SetDomainBasicAuth(zeta) error = %v", err)
	}
	if err := db.SetDomainBasicAuth(ctx, "alpha.example.com", "a-user"); err != nil {
		t.Fatalf("SetDomainBasicAuth(alpha) error = %v", err)
	}

	got, err := db.ListDomainBasicAuth(ctx)
	if err != nil {
		t.Fatalf("ListDomainBasicAuth() error = %v", err)
	}
	if len(got) != 2 || got[0].Domain != "alpha.example.com" || got[1].Domain != "zeta.example.com" {
		t.Errorf("ListDomainBasicAuth() = %+v, want alpha then zeta", got)
	}
}

func TestDomainBasicAuthSecretsKey_Stable(t *testing.T) {
	if got := DomainBasicAuthSecretsKey("app.example.com"); got != DomainBasicAuthSecretsKey("app.example.com") {
		t.Errorf("DomainBasicAuthSecretsKey() is not stable across calls: %q vs %q", got, DomainBasicAuthSecretsKey("app.example.com"))
	}
	if DomainBasicAuthSecretsKey("a.example.com") == DomainBasicAuthSecretsKey("b.example.com") {
		t.Errorf("DomainBasicAuthSecretsKey() collides across distinct domains")
	}
}
