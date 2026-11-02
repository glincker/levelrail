package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestParkedDomain_StaticSites(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []StaticSite{
		{Name: "docs", Domains: []string{"docs.example.com", "old.example.com"}, RootDir: "/srv/docs"},
		{Name: "blog", RootDir: "/srv/blog"},
	} {
		if err := db.SaveStaticSite(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	site := func(name string) DomainTarget { return DomainTarget{Kind: DomainKindStaticSite, Name: name} }

	if err := db.ParkDomainFrom(ctx, site("docs"), "old.example.com", "admin", "later"); err != nil {
		t.Fatalf("park static domain: %v", err)
	}
	p, err := db.GetParkedDomain(ctx, "old.example.com")
	if err != nil || p.ParkedFromKind != DomainKindStaticSite || p.ParkedFrom != "docs" || p.HasSettings {
		t.Fatalf("parked = %+v, %v", p, err)
	}
	if got, _ := db.GetStaticSite(ctx, "docs"); !slices.Equal(got.Domains, []string{"docs.example.com"}) {
		t.Fatalf("docs domains = %v", got.Domains)
	}
	if err := db.ParkDomainFrom(ctx, site("docs"), "old.example.com", "admin", ""); !errors.Is(err, ErrDomainNotOnApp) {
		t.Fatalf("park absent domain = %v, want ErrDomainNotOnApp", err)
	}

	// Collision: a parked hostname cannot be claimed by a static site.
	if err := db.SaveStaticSite(ctx, StaticSite{Name: "blog", Domains: []string{"old.example.com"}, RootDir: "/srv/blog"}); err == nil {
		t.Fatal("claiming a parked domain succeeded")
	}
	if _, err := db.AttachParkedDomainTo(ctx, "old.example.com", site("nope"), true); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("attach to unknown site = %v", err)
	}
	if _, err := db.AttachParkedDomainTo(ctx, "old.example.com", site("blog"), true); err != nil {
		t.Fatalf("attach to static site: %v", err)
	}
	if got, _ := db.GetStaticSite(ctx, "blog"); !slices.Equal(got.Domains, []string{"old.example.com"}) {
		t.Fatalf("blog domains = %v", got.Domains)
	}
	if _, err := db.GetParkedDomain(ctx, "old.example.com"); !errors.Is(err, ErrParkedDomainNotFound) {
		t.Fatalf("still parked: %v", err)
	}

	// Static to app, then app to static, and the collision with a taken domain.
	if err := db.MoveDomainBetweenTargets(ctx, "old.example.com", site("blog"), DomainTarget{Kind: DomainKindApp, Name: "web"}); err != nil {
		t.Fatalf("move static to app: %v", err)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); !slices.Contains(svc.Domains, "old.example.com") {
		t.Fatalf("web domains = %v", svc.Domains)
	}
	if got, _ := db.GetStaticSite(ctx, "blog"); len(got.Domains) != 0 {
		t.Fatalf("blog kept %v", got.Domains)
	}
	if err := db.MoveDomainBetweenTargets(ctx, "app.example.com", DomainTarget{Kind: DomainKindApp, Name: "web"}, site("docs")); err != nil {
		t.Fatalf("move app to static: %v", err)
	}
	if owner, found, _ := db.DomainOwnerOf(ctx, "app.example.com"); !found || owner != "docs" {
		t.Fatalf("owner = %q %v", owner, found)
	}
	var taken *ErrDomainTaken
	if err := db.SaveStaticSite(ctx, StaticSite{Name: "blog", Domains: []string{"old.example.com"}, RootDir: "/srv/blog"}); !errors.As(err, &taken) {
		t.Fatalf("static site claiming an app domain = %v, want ErrDomainTaken", err)
	}
	if err := db.MoveDomainBetweenTargets(ctx, "old.example.com", DomainTarget{Kind: DomainKindApp, Name: "web"}, DomainTarget{Kind: DomainKindStaticSite, Name: "docs", EnvID: "env_x"}); !errors.Is(err, ErrStaticSiteEnvironment) {
		t.Fatalf("static env = %v", err)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); !slices.Contains(svc.Domains, "old.example.com") {
		t.Fatalf("failed move was not rolled back: %v", svc.Domains)
	}
}
