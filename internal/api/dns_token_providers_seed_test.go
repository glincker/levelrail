package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// Every token DNS provider the code offers must have a seed row after migrations, so a provider added
// without a migration fails here instead of on a fresh install.
func TestEveryDNSTokenProviderHasASeedRow(t *testing.T) {
	t.Parallel()
	_, db, _ := newPipelineRouter(t)
	for _, name := range dnszones.TokenProviders {
		var n int
		if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM dns_token_providers WHERE provider = ?`, name).Scan(&n); err != nil {
			t.Fatalf("query seed row for %s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("dns token provider %q has %d seed rows, want 1: add it to a migration", name, n)
		}
	}
}
