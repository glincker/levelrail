package store

import (
	"context"
	"testing"
)

// A provider without a seed row (an install that predates its seed) is not configured, never an error.
func TestGetDNSTokenProviderEnabled_MissingRowIsNotConfigured(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `DELETE FROM dns_token_providers WHERE provider = 'porkbun'`); err != nil {
		t.Fatalf("delete seed row: %v", err)
	}
	enabled, err := db.GetDNSTokenProviderEnabled(ctx, "porkbun")
	if err != nil || enabled {
		t.Fatalf("missing row = (%v, %v), want (false, nil)", enabled, err)
	}
	if err := db.SetDNSTokenProviderEnabled(ctx, "hetzner", true); err != nil {
		t.Fatalf("enable seeded provider: %v", err)
	}
	if enabled, err := db.GetDNSTokenProviderEnabled(ctx, "hetzner"); err != nil || !enabled {
		t.Fatalf("seeded provider = (%v, %v), want (true, nil)", enabled, err)
	}
}
