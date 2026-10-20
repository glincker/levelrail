package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotIsValidDatabase(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	dest := filepath.Join(t.TempDir(), "snap.db")
	if _, _, err := db.SnapshotTo(ctx, dest); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	latest, err := MaxSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := InspectSnapshot(ctx, dest); err != nil || v != latest {
		t.Fatalf("inspect = %d, %v; want %d", v, err, latest)
	}
	if _, _, err := db.SnapshotTo(ctx, dest); err == nil {
		t.Fatal("expected error when destination exists")
	}
}

func TestInspectSnapshot_RejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.db")
	if err := os.WriteFile(path, []byte("not a database at all, just text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSnapshot(context.Background(), path); err == nil {
		t.Fatal("expected error for a non-database file")
	}
}

func TestOpen_DowngradeGuardAndPreMigrateHook(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := Open(ctx, path, WithPreMigrateHook(func(context.Context, *DB, int, int) {
		t.Error("hook must not run for a brand new database")
	}))
	if err != nil {
		t.Fatal(err)
	}
	latest, _ := MaxSchemaVersion()

	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES (?, 'future')`, latest+5); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := Open(ctx, path); !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("open newer db err = %v, want ErrSchemaNewer", err)
	}

	// A database stuck at version 1 has migrations pending: the hook must
	// fire before they run (the migrations themselves may then fail, since
	// the schema is not really that old, which is irrelevant here).
	old := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL DEFAULT '');
		INSERT INTO schema_migrations (version, name) VALUES (1, 'first')`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	called := false
	if db, err := Open(ctx, old, WithPreMigrateHook(func(_ context.Context, _ *DB, from, to int) {
		called = from == 1 && to == latest
	})); err == nil {
		_ = db.Close()
	}
	if !called {
		t.Error("pre-migrate hook did not run for pending migrations")
	}
}
