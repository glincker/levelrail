package store

import (
	"context"
	"errors"
	"testing"
)

func TestListAllEnvironments_SeedsAndCounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx"}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_production"); err != nil {
		t.Fatalf("tag service: %v", err)
	}
	envs, err := db.ListAllEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListAllEnvironments() error = %v", err)
	}
	if len(envs) < 4 || envs[0].ID != "env_dev" || envs[3].ID != "env_production" {
		t.Fatalf("seeds out of order: %+v", envs)
	}
	prod := envs[3]
	if prod.Kind != EnvironmentKindProduction || prod.Scope != EnvironmentScopeGlobal || !prod.Protected || prod.AppCount != 1 {
		t.Errorf("production = %+v", prod)
	}
}

func TestSaveEnvironment_KindScopeRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	e := Environment{ID: "env_x", ProjectID: GlobalProjectID, Name: "Sandbox", CreatedAt: "2026-10-06T00:00:00Z", Kind: EnvironmentKindDev, Scope: EnvironmentScopeGlobal, SortOrder: 5}
	if err := db.SaveEnvironment(ctx, e); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	got, err := db.GetEnvironment(ctx, "env_x")
	if err != nil || got != e {
		t.Fatalf("GetEnvironment() = %+v, %v; want %+v", got, err, e)
	}
}

func TestUpdateEnvironment_PartialPatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	name, kind, order, prot := "Renamed", EnvironmentKindTest, 99, true
	if err := db.UpdateEnvironment(ctx, "env_dev", EnvironmentPatch{Name: &name, Kind: &kind, SortOrder: &order, Protected: &prot}); err != nil {
		t.Fatalf("UpdateEnvironment() error = %v", err)
	}
	got, _ := db.GetEnvironment(ctx, "env_dev")
	if got.Name != name || got.Kind != kind || got.SortOrder != order || !got.Protected {
		t.Errorf("after patch = %+v", got)
	}
	only := "Again"
	if err := db.UpdateEnvironment(ctx, "env_dev", EnvironmentPatch{Name: &only}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetEnvironment(ctx, "env_dev")
	if got.Name != only || got.Kind != kind || !got.Protected {
		t.Errorf("nil fields must stay unchanged, got %+v", got)
	}
	if err := db.UpdateEnvironment(ctx, "env_nope", EnvironmentPatch{Name: &only}); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Errorf("missing id error = %v", err)
	}
}

func TestDatabaseEnvironmentAndMove(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "pg", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx"}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SetDatabaseEnvironment(ctx, "pg", "env_dev"); err != nil {
		t.Fatalf("SetDatabaseEnvironment() error = %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_dev"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDatabaseEnvironment(ctx, "missing", "env_dev"); !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("missing database error = %v", err)
	}
	names, err := db.DatabaseNamesInEnvironment(ctx, "env_dev")
	if err != nil || !names["pg"] {
		t.Fatalf("DatabaseNamesInEnvironment() = %v, %v", names, err)
	}
	apps, dbs, err := db.EnvironmentMemberCounts(ctx, "env_dev")
	if err != nil || apps != 1 || dbs != 1 {
		t.Fatalf("counts = %d, %d, %v", apps, dbs, err)
	}
	if err := db.MoveEnvironmentMembers(ctx, "env_dev", "env_test"); err != nil {
		t.Fatalf("MoveEnvironmentMembers() error = %v", err)
	}
	apps, dbs, _ = db.EnvironmentMemberCounts(ctx, "env_test")
	if apps != 1 || dbs != 1 {
		t.Errorf("after move counts = %d, %d", apps, dbs)
	}
	ref, _ := db.EnvironmentOfDatabase(ctx, "pg")
	if ref == nil || ref.ID != "env_test" {
		t.Errorf("EnvironmentOfDatabase = %+v", ref)
	}
}
