package store

import (
	"context"
	"path/filepath"
	"testing"
)

// oldSchemaCeiling is the newest migration in the v0.8.8 schema.
const oldSchemaCeiling = 3060

func TestMigrationOrder_FreshApplyRecordsEveryVersion(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("fresh open: %v", err)
	}
	defer func() { _ = db.Close() }()

	all, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	applied, err := db.appliedVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range all {
		if i > 0 && m.version <= all[i-1].version {
			t.Errorf("version %d is not greater than the previous %d", m.version, all[i-1].version)
		}
		if !applied[m.version] {
			t.Errorf("migration %04d_%s was not applied on a fresh database", m.version, m.name)
		}
	}
	for _, v := range []int{3290, 3291, 3303} {
		if !applied[v] {
			t.Errorf("migration %d missing from a fresh database", v)
		}
	}
	if err := db.MigrationsCurrent(ctx); err != nil {
		t.Errorf("MigrationsCurrent: %v", err)
	}
}

func TestMigrationOrder_UpgradeFromOldSchemaKeepsRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := Open(ctx, path, WithMigrationCeiling(oldSchemaCeiling))
	if err != nil {
		t.Fatalf("open at %d: %v", oldSchemaCeiling, err)
	}
	for _, q := range []string{
		`INSERT INTO apps (id, name) VALUES ('a1', 'web')`,
		`INSERT INTO apps (id, name) VALUES ('a2', 'api')`,
		`INSERT INTO desired_services (name, image, port, updated_at) VALUES ('web', 'img:1', 80, '2026-10-01T00:00:00Z')`,
	} {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	for _, v := range []int{3290, 3291, 3303} {
		var n int
		if err := old.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, v).Scan(&n); err != nil || n != 0 {
			t.Fatalf("old schema already has %d (n=%d err=%v)", v, n, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade from %d: %v", oldSchemaCeiling, err)
	}
	defer func() { _ = cur.Close() }()

	for tbl, want := range map[string]int{"apps": 2, "desired_services": 1} {
		var n int
		if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+tbl).Scan(&n); err != nil || n != want {
			t.Errorf("%s rows = %d (err %v), want %d", tbl, n, err, want)
		}
	}
	if err := cur.MigrationsCurrent(ctx); err != nil {
		t.Errorf("MigrationsCurrent after upgrade: %v", err)
	}
	var integrity string
	if err := cur.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Errorf("integrity_check = %q (err %v)", integrity, err)
	}
}

func TestMigrationOrder_NoDuplicateNumbers(t *testing.T) {
	all, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, m := range all {
		if prev, ok := seen[m.version]; ok {
			t.Errorf("version %d used by %s and %s", m.version, prev, m.name)
		}
		seen[m.version] = m.name
	}
}
