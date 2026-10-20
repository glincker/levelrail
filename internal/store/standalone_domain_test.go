package store

import (
	"context"
	"errors"
	"testing"
)

func TestStandaloneDomain_CRUDAndConflicts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}

	var taken *ErrDomainTaken
	err := db.CreateStandaloneDomain(ctx, StandaloneDomain{Domain: "app.example.com", Target: "127.0.0.1:3000", TargetKind: "port"})
	if !errors.As(err, &taken) || taken.Owner != "web" {
		t.Fatalf("create over an app domain = %v, want ErrDomainTaken owned by web", err)
	}

	d := StandaloneDomain{Domain: "tools.example.com", Target: "127.0.0.1:3000", TargetKind: "port", WebSocket: true, CreatedBy: "admin"}
	if err := db.CreateStandaloneDomain(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.CreateStandaloneDomain(ctx, d); !errors.As(err, &taken) || taken.Owner != StandaloneDomainOwner {
		t.Fatalf("duplicate create = %v, want ErrDomainTaken by %q", err, StandaloneDomainOwner)
	}
	err = db.SaveDesiredService(ctx, DesiredService{Name: "api", Image: "img:v1", Port: 80, Domains: []string{"tools.example.com"}})
	if !errors.As(err, &taken) || taken.Owner != StandaloneDomainOwner {
		t.Fatalf("app claiming a standalone domain = %v, want ErrDomainTaken", err)
	}

	d.Target, d.UpstreamTLS, d.TLSSkipVerify, d.ReadTimeoutSeconds = "10.0.0.5:8443", true, true, 60
	if err := db.UpdateStandaloneDomain(ctx, d); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := db.GetStandaloneDomain(ctx, "tools.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Target != "10.0.0.5:8443" || !got.UpstreamTLS || !got.TLSSkipVerify || got.ReadTimeoutSeconds != 60 || !got.WebSocket || got.CreatedBy != "admin" || got.CreatedAt.IsZero() {
		t.Fatalf("got %+v after update", got)
	}
	list, err := db.ListStandaloneDomains(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	if err := db.DeleteStandaloneDomain(ctx, "tools.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteStandaloneDomain(ctx, "tools.example.com"); !errors.Is(err, ErrStandaloneDomainNotFound) {
		t.Fatalf("second delete = %v, want not found", err)
	}
	if err := db.UpdateStandaloneDomain(ctx, d); !errors.Is(err, ErrStandaloneDomainNotFound) {
		t.Fatalf("update missing = %v, want not found", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "api", Image: "img:v1", Port: 80, Domains: []string{"tools.example.com"}}); err != nil {
		t.Fatalf("domain should be free after delete: %v", err)
	}
}

// Per-domain settings accept either owner, refuse an unowned domain and
// go away with the last owner (migration 0483's triggers).
func TestDomainSettings_FollowEitherOwner(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetDomainMaintenance(ctx, "nobody.example.com"); err == nil {
		t.Fatal("a setting for an unowned domain must be refused")
	}
	if err := db.CreateStandaloneDomain(ctx, StandaloneDomain{Domain: "tools.example.com", Target: "127.0.0.1:3000", TargetKind: "port"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"tools.example.com", "app.example.com"} {
		if err := db.SetDomainMaintenance(ctx, d); err != nil {
			t.Fatalf("maintenance on %s: %v", d, err)
		}
		if err := db.SetDomainWAF(ctx, d, true, "detect", 5, 10); err != nil {
			t.Fatalf("waf on %s: %v", d, err)
		}
	}
	if err := db.DeleteStandaloneDomain(ctx, "tools.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	left, err := db.ListDomainMaintenance(ctx)
	if err != nil || len(left) != 0 {
		t.Fatalf("maintenance rows after both owners left = %v, %v", left, err)
	}
	if waf, err := db.ListDomainWAF(ctx); err != nil || len(waf) != 0 {
		t.Fatalf("waf rows after both owners left = %v, %v", waf, err)
	}
}

func TestServerAddressOverride_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if o, err := db.GetServerAddressOverride(ctx); err != nil || o.IPv4 != "" {
		t.Fatalf("empty override = %+v, %v", o, err)
	}
	if err := db.SetServerAddressOverride(ctx, ServerAddressOverride{IPv4: "203.0.113.4", IPv6: "2001:db8::4"}); err != nil {
		t.Fatal(err)
	}
	if o, err := db.GetServerAddressOverride(ctx); err != nil || o.IPv4 != "203.0.113.4" || o.IPv6 != "2001:db8::4" {
		t.Fatalf("override = %+v, %v", o, err)
	}
	if err := db.SetServerAddressOverride(ctx, ServerAddressOverride{}); err != nil {
		t.Fatal(err)
	}
	if o, _ := db.GetServerAddressOverride(ctx); o.IPv4 != "" || o.IPv6 != "" {
		t.Fatalf("cleared override = %+v", o)
	}
}
