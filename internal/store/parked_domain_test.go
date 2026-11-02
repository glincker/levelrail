package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestParkedDomain_ParkAttachMoveDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, svc := range []DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.ParkServiceDomain(ctx, "web", "b.example.com", "admin", "later"); err != nil {
		t.Fatalf("park: %v", err)
	}
	if err := db.ParkServiceDomain(ctx, "web", "nope.example.com", "admin", ""); !errors.Is(err, ErrDomainNotOnApp) {
		t.Fatalf("park unknown = %v, want ErrDomainNotOnApp", err)
	}
	var taken *ErrDomainTaken
	err := db.SaveDesiredService(ctx, DesiredService{Name: "api", Image: "img:v1", Port: 80, Domains: []string{"b.example.com"}})
	if !errors.As(err, &taken) || taken.Owner != ParkedDomainOwner {
		t.Fatalf("claiming a parked domain = %v, want ErrDomainTaken", err)
	}
	list, err := db.ListParkedDomains(ctx)
	if err != nil || len(list) != 1 || list[0].Domain != "b.example.com" || list[0].ParkedFrom != "web" || list[0].Note != "later" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if err := db.AttachParkedDomain(ctx, "b.example.com", "api"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := db.AttachParkedDomain(ctx, "b.example.com", "api"); !errors.Is(err, ErrParkedDomainNotFound) {
		t.Fatalf("second attach = %v, want ErrParkedDomainNotFound", err)
	}
	svc, _ := db.GetDesiredService(ctx, "api")
	if !slices.Equal(svc.Domains, []string{"b.example.com"}) {
		t.Fatalf("api domains = %v", svc.Domains)
	}

	if err := db.SetDomainWAF(ctx, "a.example.com", true, DomainWAFModeBlock, 5, 5); err != nil {
		t.Fatal(err)
	}
	if err := db.MoveServiceDomain(ctx, "a.example.com", "web", "api"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if err := db.MoveServiceDomain(ctx, "a.example.com", "api", "api"); !errors.Is(err, ErrDomainOnApp) {
		t.Fatalf("move onto itself = %v, want ErrDomainOnApp", err)
	}
	if _, found, err := db.GetDomainWAF(ctx, "a.example.com"); err != nil || !found {
		t.Fatalf("WAF after move = %v, %v, want kept", found, err)
	}
	web, _ := db.GetDesiredService(ctx, "web")
	api, _ := db.GetDesiredService(ctx, "api")
	if len(web.Domains) != 0 || !slices.Equal(api.Domains, []string{"b.example.com", "a.example.com"}) {
		t.Fatalf("after move web=%v api=%v", web.Domains, api.Domains)
	}
	if owner, found, _ := db.DomainOwnerOf(ctx, "a.example.com"); !found || owner != "api" {
		t.Fatalf("owner = %q %v", owner, found)
	}

	if err := db.ParkServiceDomain(ctx, "api", "a.example.com", "admin", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteParkedDomain(ctx, "a.example.com"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := db.DeleteParkedDomain(ctx, "a.example.com"); !errors.Is(err, ErrParkedDomainNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
