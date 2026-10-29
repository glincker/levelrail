package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration3059_KeepsPreV2StatusPagesNoindex(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3058))
	if err != nil {
		t.Fatalf("open at 3058: %v", err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO status_page_settings (id, enabled, updated_at) VALUES (1, 1, '2026-10-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()
	var noindex int
	if err := cur.QueryRowContext(ctx, `SELECT noindex FROM status_page_settings WHERE id = 1`).Scan(&noindex); err != nil {
		t.Fatal(err)
	}
	if noindex != 1 {
		t.Errorf("noindex = %d after upgrade, want 1", noindex)
	}
}
