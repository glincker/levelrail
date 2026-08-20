package store

import (
	"context"
	"testing"
)

func TestGetCloudflareTunnelSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetCloudflareTunnelSettings(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareTunnelSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false on a fresh migration")
	}
}

func TestUpdateCloudflareTunnelSettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateCloudflareTunnelSettings(ctx, CloudflareTunnelSettings{Enabled: true}); err != nil {
		t.Fatalf("UpdateCloudflareTunnelSettings() error = %v", err)
	}

	got, err := db.GetCloudflareTunnelSettings(ctx)
	if err != nil {
		t.Fatalf("GetCloudflareTunnelSettings() error = %v", err)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true after update")
	}

	if err := db.UpdateCloudflareTunnelSettings(ctx, CloudflareTunnelSettings{Enabled: false}); err != nil {
		t.Fatalf("UpdateCloudflareTunnelSettings() error = %v", err)
	}
	got, err = db.GetCloudflareTunnelSettings(ctx)
	if err != nil {
		t.Fatalf("GetCloudflareTunnelSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false after disabling")
	}
}

func TestCloudflareTunnelSecretsKey_Stable(t *testing.T) {
	if got := CloudflareTunnelSecretsKey(); got != CloudflareTunnelSecretsKey() {
		t.Errorf("CloudflareTunnelSecretsKey() is not stable across calls: %q vs %q", got, CloudflareTunnelSecretsKey())
	}
}
