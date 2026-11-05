package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestParkedDomain_EnvironmentSets(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, svc := range []DesiredService{
		{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"}},
		{Name: "api", Image: "img:v1", Port: 80},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"env_stg", "env_prod"} {
		if err := db.SaveEnvironment(ctx, Environment{ID: id, ProjectID: "proj_global", Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := db.EditServiceEnvironmentDomains(ctx, "web", "env_stg", func([]string) ([]string, error) {
		return []string{"stg.example.com", "stg2.example.com"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDomainWAF(ctx, "stg.example.com", true, DomainWAFModeBlock, 5, 5); err != nil {
		t.Fatal(err)
	}

	// An env domain is not in the default list.
	if err := db.ParkServiceDomainInEnv(ctx, "web", "", "stg.example.com", "admin", ""); !errors.Is(err, ErrDomainNotOnApp) {
		t.Fatalf("park env domain from default list = %v, want ErrDomainNotOnApp", err)
	}
	if err := db.ParkServiceDomainInEnv(ctx, "web", "env_stg", "stg.example.com", "admin", "later"); err != nil {
		t.Fatalf("park env domain: %v", err)
	}
	p, err := db.GetParkedDomain(ctx, "stg.example.com")
	if err != nil || p.ParkedFromEnvironment != "env_stg" || !p.HasSettings {
		t.Fatalf("parked = %+v, %v", p, err)
	}
	// Collision: the parked hostname cannot be claimed by another app.
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "api", Image: "img:v1", Port: 80, Domains: []string{"stg.example.com"}}); err == nil {
		t.Fatal("claiming a parked env domain succeeded")
	}
	if _, err := db.AttachParkedDomainInEnv(ctx, "stg.example.com", "api", "env_nope", true); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("attach to unknown env = %v", err)
	}
	if _, err := db.AttachParkedDomainInEnv(ctx, "stg.example.com", "api", "env_prod", true); err != nil {
		t.Fatalf("attach to other app env: %v", err)
	}
	if _, found, _ := db.GetDomainWAF(ctx, "stg.example.com"); !found {
		t.Fatal("WAF settings not restored for env domain")
	}
	sets, _ := db.ListServiceEnvironmentDomains(ctx, "api")
	if !slices.Equal(sets["env_prod"], []string{"stg.example.com"}) {
		t.Fatalf("api env sets = %v", sets)
	}
	if svc, _ := db.GetDesiredService(ctx, "api"); len(svc.Domains) != 0 {
		t.Fatalf("api default list changed: %v", svc.Domains)
	}

	// Move between environments and between apps.
	if err := db.MoveServiceDomainBetween(ctx, "stg2.example.com", "web", "env_stg", "web", "env_prod"); err != nil {
		t.Fatalf("move between envs: %v", err)
	}
	if err := db.MoveServiceDomainBetween(ctx, "stg2.example.com", "web", "env_prod", "api", "env_prod"); err != nil {
		t.Fatalf("move across apps: %v", err)
	}
	if err := db.MoveServiceDomainBetween(ctx, "stg.example.com", "api", "env_prod", "api", "env_prod"); !errors.Is(err, ErrDomainOnApp) {
		t.Fatalf("move onto same set = %v, want ErrDomainOnApp", err)
	}
	if err := db.MoveServiceDomainBetween(ctx, "stg.example.com", "web", "env_stg", "api", ""); !errors.Is(err, ErrDomainNotOnApp) {
		t.Fatalf("move from wrong set = %v, want ErrDomainNotOnApp", err)
	}
	if owner, _, _ := db.DomainOwnerOf(ctx, "stg2.example.com"); owner != "api" {
		t.Fatalf("owner after move = %q", owner)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); !slices.Equal(svc.Domains, []string{"www.example.com"}) {
		t.Fatalf("web default list changed: %v", svc.Domains)
	}
}
