package store

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestSaveStaticSite_SaveAndList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	site := StaticSite{Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/static/docs/abc123"}
	if err := db.SaveStaticSite(ctx, site); err != nil {
		t.Fatalf("SaveStaticSite() error = %v", err)
	}

	sites, err := db.ListStaticSites(ctx)
	if err != nil {
		t.Fatalf("ListStaticSites() error = %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("ListStaticSites() = %d sites, want 1", len(sites))
	}
	if !reflect.DeepEqual(sites[0], site) {
		t.Errorf("ListStaticSites()[0] = %+v, want %+v", sites[0], site)
	}
}

func TestSaveStaticSite_Redeploy_ReplacesWholeRecord(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/static/docs/v1",
	}); err != nil {
		t.Fatalf("initial SaveStaticSite() error = %v", err)
	}

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/static/docs/v2",
	}); err != nil {
		t.Fatalf("redeploy SaveStaticSite() error = %v", err)
	}

	sites, err := db.ListStaticSites(ctx)
	if err != nil {
		t.Fatalf("ListStaticSites() error = %v", err)
	}
	if len(sites) != 1 || sites[0].RootDir != "/data/static/docs/v2" {
		t.Errorf("ListStaticSites() = %+v, want exactly one site with RootDir=/data/static/docs/v2", sites)
	}
}

func TestSaveStaticSite_ListOrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	names := []string{"zeta", "alpha", "mu"}
	for _, n := range names {
		if err := db.SaveStaticSite(ctx, StaticSite{Name: n, RootDir: "/data/static/" + n}); err != nil {
			t.Fatalf("SaveStaticSite(%s) error = %v", n, err)
		}
	}

	sites, err := db.ListStaticSites(ctx)
	if err != nil {
		t.Fatalf("ListStaticSites() error = %v", err)
	}
	got := make([]string, len(sites))
	for i, s := range sites {
		got[i] = s.Name
	}
	want := append([]string(nil), names...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListStaticSites() names = %v, want %v", got, want)
	}
}

func TestSaveStaticSite_DomainTaken_ByAnotherStaticSite_RejectsWrite(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"shared.example.com"}, RootDir: "/data/static/docs",
	}); err != nil {
		t.Fatalf("SaveStaticSite(docs) error = %v", err)
	}

	err := db.SaveStaticSite(ctx, StaticSite{
		Name: "blog", Domains: []string{"shared.example.com"}, RootDir: "/data/static/blog",
	})
	var taken *ErrDomainTaken
	if !errors.As(err, &taken) {
		t.Fatalf("SaveStaticSite(blog) error = %v, want *ErrDomainTaken", err)
	}
	if taken.Domain != "shared.example.com" || taken.Owner != "docs" {
		t.Errorf("ErrDomainTaken = %+v, want Domain=shared.example.com Owner=docs", taken)
	}

	sites, err := db.ListStaticSites(ctx)
	if err != nil {
		t.Fatalf("ListStaticSites() error = %v", err)
	}
	if len(sites) != 1 {
		t.Errorf("ListStaticSites() = %+v, want only docs: the rejected write must not partially create blog", sites)
	}
}

// TestSaveStaticSite_DomainTaken_ByContainerService and its mirror below
// are the cross-table conflict this migration exists for
// (migrations/0015's own comment): a static site and a container
// service must never both claim the same domain, since
// internal/reconcile/ingress's controller would then build two
// conflicting Caddy routes for the same host, one a reverse_proxy, one
// a file_server.
func TestSaveStaticSite_DomainTaken_ByContainerService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"shared.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}

	err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"shared.example.com"}, RootDir: "/data/static/docs",
	})
	var taken *ErrDomainTaken
	if !errors.As(err, &taken) {
		t.Fatalf("SaveStaticSite(docs) error = %v, want *ErrDomainTaken", err)
	}
	if taken.Domain != "shared.example.com" || taken.Owner != "web" {
		t.Errorf("ErrDomainTaken = %+v, want Domain=shared.example.com Owner=web", taken)
	}
}

func TestSaveDesiredService_DomainTaken_ByStaticSite(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"shared.example.com"}, RootDir: "/data/static/docs",
	}); err != nil {
		t.Fatalf("SaveStaticSite(docs) error = %v", err)
	}

	err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"shared.example.com"},
	})
	var taken *ErrDomainTaken
	if !errors.As(err, &taken) {
		t.Fatalf("SaveDesiredService(web) error = %v, want *ErrDomainTaken", err)
	}
	if taken.Domain != "shared.example.com" || taken.Owner != "docs" {
		t.Errorf("ErrDomainTaken = %+v, want Domain=shared.example.com Owner=docs", taken)
	}
}

func TestSaveStaticSite_DomainReleasedWhenRedeployedWithout(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"a.example.com", "b.example.com"}, RootDir: "/data/static/docs/v1",
	}); err != nil {
		t.Fatalf("initial SaveStaticSite() error = %v", err)
	}

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "docs", Domains: []string{"a.example.com"}, RootDir: "/data/static/docs/v2",
	}); err != nil {
		t.Fatalf("redeploy dropping a domain error = %v", err)
	}

	if err := db.SaveStaticSite(ctx, StaticSite{
		Name: "blog", Domains: []string{"b.example.com"}, RootDir: "/data/static/blog",
	}); err != nil {
		t.Errorf("SaveStaticSite(blog) claiming a domain docs released error = %v, want nil", err)
	}
}
