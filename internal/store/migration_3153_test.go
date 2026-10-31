package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration3153_ExistingOAuthRowsHaveNoTenant(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(3152))
	if err != nil {
		t.Fatalf("open at 3152: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()
	s, err := cur.GetOAuthProviderSettings(ctx, OAuthProviderMicrosoft)
	if err != nil {
		t.Fatal(err)
	}
	if s.TenantID != "" {
		t.Errorf("tenant id = %q after upgrade, want empty", s.TenantID)
	}
}
