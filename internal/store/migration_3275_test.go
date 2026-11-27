package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigration3275_ImageRepoBackfillOldestWinsAndIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3274))
	if err != nil {
		t.Fatalf("open at 3274: %v", err)
	}
	for _, s := range []struct{ name, image string }{
		{"web", "web:0123456789abcdef0123456789abcdef01234567"},
		{"clone", "docker.io/library/web:0123456789abcdef0123456789abcdef01234567"},
		{"cache", "redis:7"},
	} {
		// Raw insert: the save SQL names the rev column added after this ceiling.
		if _, err := old.ExecContext(ctx, `INSERT INTO desired_services (name, image, port, updated_at) VALUES (?, ?, 80, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, s.name, s.image); err != nil {
			t.Fatalf("seed %s: %v", s.name, err)
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
	if got, _ := cur.ImageRepoOwner(ctx, "docker.io/library/web"); got != "web" {
		t.Errorf("web repo owner = %q, want the oldest app web", got)
	}
	if got, _ := cur.ImageRepoOwner(ctx, "docker.io/library/cache"); got != "cache" {
		t.Errorf("cache repo owner = %q, want cache", got)
	}
	sqlText, err := os.ReadFile(filepath.Join("migrations", "3275_image_repo_owners_backfill.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	_ = cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_repo_owners`).Scan(&before)
	if _, err := cur.ExecContext(ctx, string(sqlText)); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	_ = cur.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_repo_owners`).Scan(&after)
	if before != after || before != 2 {
		t.Errorf("rows before=%d after=%d, want 2 and unchanged", before, after)
	}
}
