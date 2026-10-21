package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHeldStaticBuild(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	site := StaticSite{Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/docs/abc"}

	if err := db.SaveHeldStaticBuild(ctx, "d1", site, now); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveHeldBuild(ctx, "d2", DesiredService{Name: "docs", Image: "x"}, now); err != nil {
		t.Fatal(err)
	}
	if ids, _ := db.ListHeldBuildIDs(ctx, "docs"); !ids["d1"] || !ids["d2"] {
		t.Fatalf("held ids = %v", ids)
	}
	got, err := db.GetHeldStaticBuild(ctx, "d1")
	if err != nil || got.RootDir != site.RootDir || got.Domains[0] != "docs.example.com" {
		t.Fatalf("GetHeldStaticBuild = %+v, %v", got, err)
	}
	if _, err := db.GetHeldBuild(ctx, "d1"); !errors.Is(err, ErrHeldBuildNotFound) {
		t.Fatalf("static build read as a desired service: %v", err)
	}
	if _, err := db.GetHeldStaticBuild(ctx, "d2"); !errors.Is(err, ErrHeldBuildNotFound) {
		t.Fatalf("desired-service build read as a static site: %v", err)
	}
	if err := db.MarkHeldBuildPublished(ctx, "d1", "x", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetHeldStaticBuild(ctx, "d1"); !errors.Is(err, ErrHeldBuildNotFound) {
		t.Fatalf("published build still readable: %v", err)
	}
}
