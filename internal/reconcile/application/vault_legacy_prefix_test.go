package application

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCheckLegacyVaultPaths(t *testing.T) {
	ctx := context.Background()
	c := &Controller{legacyVaultPrefixes: func(context.Context) []string { return []string{"secret/data/team-a"} }}

	if err := c.checkLegacyVaultPaths(ctx, map[string]store.VaultEnvRef{"A": {Path: "secret/data/team-a/db", Key: "k", Unvetted: true}}); err != nil {
		t.Fatalf("inside the allowlist: %v", err)
	}
	if err := c.checkLegacyVaultPaths(ctx, map[string]store.VaultEnvRef{"A": {Path: "old/path", Key: "k"}}); err != nil {
		t.Fatalf("a vetted stored path must keep resolving: %v", err)
	}
	err := c.checkLegacyVaultPaths(ctx, map[string]store.VaultEnvRef{"A": {Path: "secret/data/team-b/db", Key: "k", Unvetted: true}})
	if err == nil || !strings.Contains(err.Error(), "vault.legacy_allowed_prefixes") {
		t.Fatalf("an unvetted path outside the allowlist must be refused, got %v", err)
	}
}
