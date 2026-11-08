package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

// lastV092Migration is the newest migration shipped in tag v0.9.2.
const lastV092Migration = 3199

func TestMigrationsPopulated_UpgradeFromV092(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(lastV092Migration))
	if err != nil {
		t.Fatalf("open at %d: %v", lastV092Migration, err)
	}
	seed := []string{
		`INSERT INTO users (id, email, display_name, password_hash, created_at) VALUES ('u1', 'a@example.test', 'A', 'test-hash', '2026-10-01T00:00:00Z')`,
		`INSERT INTO apps (id, name) VALUES ('a1', 'web')`,
		`INSERT INTO service_secrets (service_name, wrapped_dek) VALUES ('web', x'01020304')`,
		`INSERT INTO service_secret_values (service_name, env_key, ciphertext) VALUES ('web', 'KEY', CAST('test-secret-ciphertext' AS BLOB))`,
		`INSERT INTO backup_targets (id, name, provider, bucket, created_at) VALUES ('t1', 'main', 'aws', 'bkt', '2026-10-01T00:00:00Z')`,
		`INSERT INTO secret_rotation_policies (service_name, env_key, interval_days, created_at, updated_at) VALUES ('web', 'KEY', 30, '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z')`,
	}
	for _, q := range seed {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()

	for _, tbl := range []string{"users", "apps", "service_secrets", "service_secret_values", "backup_targets", "secret_rotation_policies"} {
		var n int
		if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+tbl).Scan(&n); err != nil || n != 1 {
			t.Errorf("%s rows = %d (err %v), want 1", tbl, n, err)
		}
	}
	var ct, dek []byte
	if err := cur.QueryRowContext(ctx, `SELECT ciphertext FROM service_secret_values WHERE service_name='web'`).Scan(&ct); err != nil {
		t.Fatal(err)
	}
	if err := cur.QueryRowContext(ctx, `SELECT wrapped_dek FROM service_secrets WHERE service_name='web'`).Scan(&dek); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ct, []byte("test-secret-ciphertext")) || !bytes.Equal(dek, []byte{1, 2, 3, 4}) {
		t.Errorf("secret columns changed: %q %x", ct, dek)
	}
	var nulls int
	if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email IS NULL OR display_name IS NULL OR created_at IS NULL`).Scan(&nulls); err != nil || nulls != 0 {
		t.Errorf("users NOT NULL columns have %d nulls (err %v)", nulls, err)
	}

	fk, err := cur.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if fk.Next() {
		t.Error("foreign_key_check reported violations")
	}
	_ = fk.Close()
	var integrity string
	if err := cur.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Errorf("integrity_check = %q (err %v)", integrity, err)
	}

	all, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	applied, err := cur.appliedVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(all) {
		t.Errorf("applied %d versions, embedded %d", len(applied), len(all))
	}
	for _, m := range all {
		if !applied[m.version] {
			t.Errorf("migration %d not applied", m.version)
		}
	}
	if p := pendingMigrations(all, applied); len(p) != 0 {
		t.Errorf("%d migrations still pending", len(p))
	}
	if err := cur.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	_ = again.Close()
}
