package store

import (
	"context"
	"errors"
	"testing"
)

func TestSaveAndGetDesiredDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}
	if err := db.SaveDesiredDatabase(ctx, want); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if *got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSaveDesiredDatabase_UpsertReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "15"}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.Version != "16" {
		t.Errorf("Version = %q, want 16 (the second save's value)", got.Version)
	}

	all, err := db.ListDesiredDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDesiredDatabases() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected exactly 1 row after two saves to the same name, got %d", len(all))
	}
}

func TestGetDesiredDatabase_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDesiredDatabase(context.Background(), "never-saved")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestListDesiredDatabases_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"cache", "main", "analytics"} {
		if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: name, Engine: EngineRedis}); err != nil {
			t.Fatalf("save %q: %v", name, err)
		}
	}

	got, err := db.ListDesiredDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDesiredDatabases() error = %v", err)
	}
	want := []string{"analytics", "cache", "main"}
	if len(got) != len(want) {
		t.Fatalf("expected %d databases, got %d", len(want), len(got))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: got %q, want %q (expected alphabetical order)", i, got[i].Name, name)
		}
	}
}

func TestUpdateDatabaseNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node-1"); err != nil {
		t.Fatalf("UpdateDatabaseNode() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1", got.NodeID)
	}
}

func TestUpdateDatabaseNode_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateDatabaseNode(context.Background(), "nonexistent", "node-1")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("UpdateDatabaseNode() error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestSaveDesiredDatabase_ResaveDoesNotResetNodeID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "15"}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node-1"); err != nil {
		t.Fatalf("UpdateDatabaseNode() error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("resave: %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.Version != "16" {
		t.Errorf("Version = %q, want 16", got.Version)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1 (a resave must not silently un-assign a placed database)", got.NodeID)
	}
}
