package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})
	return db
}

func TestOpen_RunsMigrationsAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "levelrail.db")
	ctx := context.Background()

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}

	var version int
	if err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if version < 1 {
		t.Fatalf("expected at least migration version 1 applied, got %d", version)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopening the same file must not fail or re-apply anything, this is
	// the normal "restart the control plane binary" path.
	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() on existing db error = %v", err)
	}
	defer func() {
		if err := db2.Close(); err != nil {
			t.Errorf("closing db2: %v", err)
		}
	}()

	var count int
	if err := db2.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations after reopen: %v", err)
	}
	if count != 1 {
		t.Errorf("expected migration %d recorded exactly once after reopen, got %d rows", version, count)
	}
}

func TestOpen_WALModeEnabled(t *testing.T) {
	db := openTestDB(t)
	var mode string
	if err := db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q (CLAUDE.md 4.7 requires WAL mode)", mode, "wal")
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	db := openTestDB(t)
	var enabled int
	if err := db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys pragma = %d, want 1 (enabled)", enabled)
	}
}
