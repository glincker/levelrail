package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedDatabaseForInitScripts(t *testing.T, db *DB, name string) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), DesiredDatabase{Name: name, Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase(%q) error = %v", name, err)
	}
}

func testDatabaseInitScript() DatabaseInitScript {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	return DatabaseInitScript{
		ID: "dis_test1", DatabaseName: "mydb", Filename: "01-extensions.sql",
		Content: "CREATE EXTENSION IF NOT EXISTS vector;", CreatedAt: now, UpdatedAt: now,
	}
}

func TestSaveAndGetDatabaseInitScript(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDatabaseForInitScripts(t, db, "mydb")
	want := testDatabaseInitScript()

	if err := db.SaveDatabaseInitScript(ctx, want); err != nil {
		t.Fatalf("SaveDatabaseInitScript() error = %v", err)
	}

	got, err := db.GetDatabaseInitScript(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetDatabaseInitScript() error = %v", err)
	}
	if got != want {
		t.Errorf("GetDatabaseInitScript() = %+v, want %+v", got, want)
	}
}

func TestGetDatabaseInitScript_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDatabaseInitScript(context.Background(), "missing")
	if !errors.Is(err, ErrDatabaseInitScriptNotFound) {
		t.Fatalf("GetDatabaseInitScript() error = %v, want ErrDatabaseInitScriptNotFound", err)
	}
}

func TestListDatabaseInitScripts_OrderedByFilename(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDatabaseForInitScripts(t, db, "mydb")
	seedDatabaseForInitScripts(t, db, "other")

	second := testDatabaseInitScript()
	second.ID, second.Filename = "dis_2", "02-roles.sql"
	first := testDatabaseInitScript()
	first.ID, first.Filename = "dis_1", "01-extensions.sql"
	unrelated := testDatabaseInitScript()
	unrelated.ID, unrelated.DatabaseName, unrelated.Filename = "dis_3", "other", "01-other.sql"

	// Saved out of filename order, to prove the query itself orders
	// results rather than happening to reflect insertion order.
	for _, s := range []DatabaseInitScript{second, first, unrelated} {
		if err := db.SaveDatabaseInitScript(ctx, s); err != nil {
			t.Fatalf("SaveDatabaseInitScript(%q) error = %v", s.ID, err)
		}
	}

	got, err := db.ListDatabaseInitScripts(ctx, "mydb")
	if err != nil {
		t.Fatalf("ListDatabaseInitScripts() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListDatabaseInitScripts() returned %d rows, want 2", len(got))
	}
	if got[0].Filename != "01-extensions.sql" || got[1].Filename != "02-roles.sql" {
		t.Errorf("order = [%q, %q], want [01-extensions.sql, 02-roles.sql]", got[0].Filename, got[1].Filename)
	}
}

func TestUpdateDatabaseInitScript(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDatabaseForInitScripts(t, db, "mydb")
	seeded := testDatabaseInitScript()
	if err := db.SaveDatabaseInitScript(ctx, seeded); err != nil {
		t.Fatalf("SaveDatabaseInitScript() error = %v", err)
	}

	updatedAt := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if err := db.UpdateDatabaseInitScript(ctx, seeded.ID, "01-extensions-updated.sql", "CREATE EXTENSION IF NOT EXISTS postgis;", updatedAt); err != nil {
		t.Fatalf("UpdateDatabaseInitScript() error = %v", err)
	}

	got, err := db.GetDatabaseInitScript(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("GetDatabaseInitScript() error = %v", err)
	}
	if got.Filename != "01-extensions-updated.sql" || got.Content != "CREATE EXTENSION IF NOT EXISTS postgis;" {
		t.Errorf("got = %+v, want updated filename/content", got)
	}
	if got.DatabaseName != "mydb" {
		t.Errorf("DatabaseName = %q, changed by an update that must never touch it", got.DatabaseName)
	}
}

func TestUpdateDatabaseInitScript_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateDatabaseInitScript(context.Background(), "missing", "a.sql", "x", time.Now())
	if !errors.Is(err, ErrDatabaseInitScriptNotFound) {
		t.Fatalf("UpdateDatabaseInitScript() error = %v, want ErrDatabaseInitScriptNotFound", err)
	}
}

func TestDeleteDatabaseInitScript(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDatabaseForInitScripts(t, db, "mydb")
	seeded := testDatabaseInitScript()
	if err := db.SaveDatabaseInitScript(ctx, seeded); err != nil {
		t.Fatalf("SaveDatabaseInitScript() error = %v", err)
	}

	if err := db.DeleteDatabaseInitScript(ctx, seeded.ID); err != nil {
		t.Fatalf("DeleteDatabaseInitScript() error = %v", err)
	}
	if _, err := db.GetDatabaseInitScript(ctx, seeded.ID); !errors.Is(err, ErrDatabaseInitScriptNotFound) {
		t.Fatalf("GetDatabaseInitScript() after delete error = %v, want ErrDatabaseInitScriptNotFound", err)
	}
}

func TestDeleteDatabaseInitScript_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.DeleteDatabaseInitScript(context.Background(), "missing")
	if !errors.Is(err, ErrDatabaseInitScriptNotFound) {
		t.Fatalf("DeleteDatabaseInitScript() error = %v, want ErrDatabaseInitScriptNotFound", err)
	}
}

func TestDeleteDesiredDatabase_CascadesInitScripts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedDatabaseForInitScripts(t, db, "mydb")
	seeded := testDatabaseInitScript()
	if err := db.SaveDatabaseInitScript(ctx, seeded); err != nil {
		t.Fatalf("SaveDatabaseInitScript() error = %v", err)
	}

	if err := db.DeleteDesiredDatabase(ctx, "mydb"); err != nil {
		t.Fatalf("DeleteDesiredDatabase() error = %v", err)
	}

	if _, err := db.GetDatabaseInitScript(ctx, seeded.ID); !errors.Is(err, ErrDatabaseInitScriptNotFound) {
		t.Fatalf("GetDatabaseInitScript() after database delete error = %v, want ErrDatabaseInitScriptNotFound (ON DELETE CASCADE)", err)
	}
}
