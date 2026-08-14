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

// TestSaveAndGetDesiredDatabase_MySQL reproduces the real bug migration
// 0016 fixes: desired_databases.engine originally carried a hardcoded
// CHECK (engine IN ('postgres', 'redis')) (0003_desired_databases.sql),
// so a mysql row failed at the SQLite layer even though every
// application-layer check (validateDatabaseResource,
// SupportedDatabaseEngines) already accepted it. Confirmed live via a
// real browser click-through before this test existed: "CHECK
// constraint failed: engine IN ('postgres', 'redis')".
func TestSaveAndGetDesiredDatabase_MySQL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredDatabase{Name: "orders", Engine: EngineMySQL, Version: "8"}
	if err := db.SaveDesiredDatabase(ctx, want); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "orders")
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

func TestDeleteDesiredDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.DeleteDesiredDatabase(ctx, "main"); err != nil {
		t.Fatalf("DeleteDesiredDatabase() error = %v", err)
	}

	if _, err := db.GetDesiredDatabase(ctx, "main"); !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("GetDesiredDatabase after delete: error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestDeleteDesiredDatabase_NotFound(t *testing.T) {
	db := openTestDB(t)

	err := db.DeleteDesiredDatabase(context.Background(), "never-saved")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestDeleteDesiredDatabase_DoesNotAffectOtherDatabases(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"cache", "main"} {
		if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: name, Engine: EngineRedis, Version: "7"}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}

	if err := db.DeleteDesiredDatabase(ctx, "main"); err != nil {
		t.Fatalf("DeleteDesiredDatabase() error = %v", err)
	}

	if _, err := db.GetDesiredDatabase(ctx, "cache"); err != nil {
		t.Errorf("unrelated database cache should be untouched, got error = %v", err)
	}
}

// TestListDesiredDatabasesByNode is the database-kind counterpart to
// TestListDesiredServicesByNode (service_test.go), same TASKS.md 3.7
// drain/delete-guard callers.
func TestListDesiredDatabasesByNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, d := range []DesiredDatabase{
		{Name: "main", Engine: EngineRedis, Version: "7"},
		{Name: "cache", Engine: EngineRedis, Version: "7"},
	} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatalf("SaveDesiredDatabase(%s) error = %v", d.Name, err)
		}
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node-1"); err != nil {
		t.Fatalf("UpdateDatabaseNode(main) error = %v", err)
	}
	// cache stays on the local node ("").

	got, err := db.ListDesiredDatabasesByNode(ctx, "node-1")
	if err != nil {
		t.Fatalf("ListDesiredDatabasesByNode(node-1) error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "main" {
		t.Fatalf("ListDesiredDatabasesByNode(node-1) = %+v, want [main]", got)
	}

	gotEmpty, err := db.ListDesiredDatabasesByNode(ctx, "node-2")
	if err != nil {
		t.Fatalf("ListDesiredDatabasesByNode(node-2) error = %v", err)
	}
	if len(gotEmpty) != 0 {
		t.Errorf("ListDesiredDatabasesByNode(node-2) = %+v, want empty", gotEmpty)
	}
}
