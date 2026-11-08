package store

import (
	"context"
	"testing"
)

func TestDomainLimits_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.GetDomainLimits(ctx, "app.example.com"); err != nil || found {
		t.Fatalf("before set = found %v, err %v", found, err)
	}
	want := DomainLimits{Domain: "app.example.com", MaxBodyBytes: 5 << 20, ReadTimeoutSeconds: 30, WriteTimeoutSeconds: 45}
	if err := db.SetDomainLimits(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := db.GetDomainLimits(ctx, "app.example.com")
	if err != nil || !found || got.UpdatedAt.IsZero() {
		t.Fatalf("get = %+v, found %v, err %v", got, found, err)
	}
	got.UpdatedAt = want.UpdatedAt
	if got != want {
		t.Errorf("get = %+v, want %+v", got, want)
	}
	want.MaxBodyBytes = 0
	if err := db.SetDomainLimits(ctx, want); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListDomainLimits(ctx)
	if err != nil || len(list) != 1 || list[0].MaxBodyBytes != 0 || list[0].ReadTimeoutSeconds != 30 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteDomainLimits(ctx, "app.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDomainLimits(ctx, "app.example.com"); err != nil {
		t.Fatalf("second delete = %v, want idempotent", err)
	}
	if _, found, _ := db.GetDomainLimits(ctx, "app.example.com"); found {
		t.Error("limits still present after delete")
	}
}

func TestDomainLimits_OwnerTriggers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetDomainLimits(ctx, DomainLimits{Domain: "nobody.example.com", ReadTimeoutSeconds: 5}); err == nil {
		t.Fatal("SetDomainLimits() for an unowned domain succeeded, want the owner trigger to refuse it")
	}

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateStandaloneDomain(ctx, StandaloneDomain{Domain: "ext.example.com", Target: "127.0.0.1:3000", TargetKind: "port"}); err != nil {
		t.Fatalf("CreateStandaloneDomain() error = %v", err)
	}
	if err := db.SaveStaticSite(ctx, StaticSite{Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/srv/docs"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"app.example.com", "ext.example.com", "docs.example.com"} {
		if err := db.SetDomainLimits(ctx, DomainLimits{Domain: d, MaxBodyBytes: 1 << 20, WriteTimeoutSeconds: 9}); err != nil {
			t.Fatalf("SetDomainLimits(%s) error = %v", d, err)
		}
	}

	if err := db.DeleteStaticSite(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteStandaloneDomain(ctx, "ext.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListDomainLimits(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("list after owners are gone = %+v, %v; want none", list, err)
	}
}
