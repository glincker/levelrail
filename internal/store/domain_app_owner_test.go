package store

import (
	"context"
	"testing"
)

func TestDomainAppOwnerOf_IgnoresStaticSiteWithAppName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveStaticSite(ctx, StaticSite{Name: "web", Domains: []string{"site.example.com"}, RootDir: "/srv/web"}); err != nil {
		t.Fatal(err)
	}
	if app, found, err := db.DomainAppOwnerOf(ctx, "app.example.com"); err != nil || !found || app != "web" {
		t.Errorf("app domain = %q %v %v", app, found, err)
	}
	if app, found, _ := db.DomainAppOwnerOf(ctx, "site.example.com"); found {
		t.Errorf("static site domain resolved to app %q", app)
	}
}
