package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration1300_RekeysNodeTagsByID(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(1280))
	if err != nil {
		t.Fatalf("open at 1280: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO nodes (id, name) VALUES ('n1', 'edge-a')`,
		`INSERT INTO tags (id, name, created_at) VALUES ('t1', 'prod', '2026-01-01T00:00:00Z')`,
		`INSERT INTO resource_tags (tag_id, kind, name, created_at) VALUES ('t1', 'node', 'edge-a', 'x')`,
		`INSERT INTO resource_tags (tag_id, kind, name, created_at) VALUES ('t1', 'domain', 'edge-a', 'x')`,
		`INSERT INTO resource_tags (tag_id, kind, name, created_at) VALUES ('t1', 'node', 'gone', 'x')`,
	} {
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
	for _, c := range []struct{ kind, name string }{{"node", "n1"}, {"domain", "edge-a"}, {"node", "gone"}} {
		var n int
		if err := cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM resource_tags WHERE kind = ? AND name = ?`, c.kind, c.name).Scan(&n); err != nil || n != 1 {
			t.Errorf("%s:%s rows = %d err = %v, want 1", c.kind, c.name, n, err)
		}
	}
}
