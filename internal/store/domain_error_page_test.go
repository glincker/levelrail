package store

import (
	"context"
	"testing"
)

func TestDomainErrorPage_SetGetClear_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if got, err := db.ListDomainErrorPages(ctx, "www.example.com"); err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	} else if len(got) != 0 {
		t.Errorf("ListDomainErrorPages() = %+v, want none before any are set", got)
	}

	if err := db.SetDomainErrorPage(ctx, "www.example.com", DomainErrorPageNotFound, "<h1>not found</h1>"); err != nil {
		t.Fatalf("SetDomainErrorPage(404) error = %v", err)
	}
	if err := db.SetDomainErrorPage(ctx, "www.example.com", DomainErrorPageServiceUnavailable, "<h1>down</h1>"); err != nil {
		t.Fatalf("SetDomainErrorPage(503) error = %v", err)
	}

	got, err := db.ListDomainErrorPages(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(got) != 2 || got[0].StatusCode != DomainErrorPageNotFound || got[1].StatusCode != DomainErrorPageServiceUnavailable {
		t.Fatalf("ListDomainErrorPages() = %+v, want 404 then 503 (ordered by status code)", got)
	}
	if got[0].Body != "<h1>not found</h1>" {
		t.Errorf("pages[0].Body = %q, want the 404 body", got[0].Body)
	}

	// Setting the same status code again overwrites, not errors or adds
	// a second row.
	if err := db.SetDomainErrorPage(ctx, "www.example.com", DomainErrorPageNotFound, "<h1>updated</h1>"); err != nil {
		t.Fatalf("SetDomainErrorPage() (overwrite) error = %v", err)
	}
	got, err = db.ListDomainErrorPages(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(got) != 2 || got[0].Body != "<h1>updated</h1>" {
		t.Errorf("ListDomainErrorPages() = %+v, want the 404 row overwritten, not duplicated", got)
	}

	if err := db.DeleteDomainErrorPage(ctx, "www.example.com", DomainErrorPageNotFound); err != nil {
		t.Fatalf("DeleteDomainErrorPage(404) error = %v", err)
	}
	got, err = db.ListDomainErrorPages(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(got) != 1 || got[0].StatusCode != DomainErrorPageServiceUnavailable {
		t.Errorf("ListDomainErrorPages() = %+v, want only the 503 row left", got)
	}

	// Idempotent: deleting an already-cleared mapping is not an error.
	if err := db.DeleteDomainErrorPage(ctx, "www.example.com", DomainErrorPageNotFound); err != nil {
		t.Fatalf("DeleteDomainErrorPage() (already cleared) error = %v", err)
	}

	if err := db.DeleteDomainErrorPages(ctx, "www.example.com"); err != nil {
		t.Fatalf("DeleteDomainErrorPages() error = %v", err)
	}
	if got, err := db.ListDomainErrorPages(ctx, "www.example.com"); err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	} else if len(got) != 0 {
		t.Errorf("ListDomainErrorPages() = %+v, want none after DeleteDomainErrorPages", got)
	}

	// Idempotent: clearing an already-empty domain is not an error.
	if err := db.DeleteDomainErrorPages(ctx, "www.example.com"); err != nil {
		t.Fatalf("DeleteDomainErrorPages() (already cleared) error = %v", err)
	}
}

func TestDomainErrorPage_CascadesWhenDomainRemoved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainErrorPage(ctx, "www.example.com", DomainErrorPageNotFound, "<h1>not found</h1>"); err != nil {
		t.Fatalf("SetDomainErrorPage() error = %v", err)
	}

	// Redeploying web with no domains removes its service_domains row,
	// which must cascade to drop every error page row for it.
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domain) error = %v", err)
	}

	got, err := db.ListDomainErrorPages(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListDomainErrorPages() = %+v, want none: rows should cascade-delete with their domain", got)
	}
}

func TestListAllDomainErrorPages_OrderedByDomainThenStatusCode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"zeta.example.com", "alpha.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SetDomainErrorPage(ctx, "zeta.example.com", DomainErrorPageServerError, "z-500"); err != nil {
		t.Fatalf("SetDomainErrorPage(zeta, 500) error = %v", err)
	}
	if err := db.SetDomainErrorPage(ctx, "alpha.example.com", DomainErrorPageBadGateway, "a-502"); err != nil {
		t.Fatalf("SetDomainErrorPage(alpha, 502) error = %v", err)
	}
	if err := db.SetDomainErrorPage(ctx, "alpha.example.com", DomainErrorPageNotFound, "a-404"); err != nil {
		t.Fatalf("SetDomainErrorPage(alpha, 404) error = %v", err)
	}

	got, err := db.ListAllDomainErrorPages(ctx)
	if err != nil {
		t.Fatalf("ListAllDomainErrorPages() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListAllDomainErrorPages() = %+v, want 3 rows", got)
	}
	want := []DomainErrorPage{
		{Domain: "alpha.example.com", StatusCode: DomainErrorPageNotFound, Body: "a-404"},
		{Domain: "alpha.example.com", StatusCode: DomainErrorPageBadGateway, Body: "a-502"},
		{Domain: "zeta.example.com", StatusCode: DomainErrorPageServerError, Body: "z-500"},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("ListAllDomainErrorPages()[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}
