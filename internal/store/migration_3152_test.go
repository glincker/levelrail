package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration3152_ExistingGitSourcesKeepAppAuth(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3151))
	if err != nil {
		t.Fatalf("open at 3151: %v", err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO service_git_sources (service_name, repo_url, branch, build_type, build_path, updated_at) VALUES ('web', 'https://example.com/a.git', 'main', 'dockerfile', 'Dockerfile', '2026-10-01T00:00:00Z')`); err != nil {
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
	var v int
	if err := cur.QueryRowContext(ctx, `SELECT app_auth FROM service_git_sources WHERE service_name = 'web'`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Errorf("app_auth = %d after upgrade, want 1", v)
	}
}
