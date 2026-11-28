package application

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// A retrying reconcile stores the refusal once and never stores it for an allowed path.
func TestLegacyVaultRefusalStoresOneCoalescedEvent(t *testing.T) {
	ctx := context.Background()
	st := &eventFakeStore{}
	c := New("web", st, newFakeRuntime(0), WithLegacyVaultPrefixes(func(context.Context) []string { return []string{"secret/data/team-a"} }))
	allowed := map[string]store.VaultEnvRef{"A": {Path: "secret/data/team-a/db", Key: "k", Unvetted: true}}
	refused := map[string]store.VaultEnvRef{"A": {Path: "secret/data/team-b/db", Key: "k", Unvetted: true}}

	if err := c.checkLegacyVaultPaths(ctx, allowed); err != nil || len(st.events) != 0 {
		t.Fatalf("allowed path: err = %v, events = %+v", err, st.events)
	}
	for i := 0; i < 4; i++ {
		if err := c.checkLegacyVaultPaths(ctx, refused); err == nil {
			t.Fatalf("pass %d must refuse", i)
		}
	}
	if len(st.events) != 1 || st.events[0].Kind != store.AppEventLegacyVaultPathRefused || st.events[0].AppName != "web" {
		t.Fatalf("events = %+v, want one legacy_vault_path_refused event for web", st.events)
	}
}

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
