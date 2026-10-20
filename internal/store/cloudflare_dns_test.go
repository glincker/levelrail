package store

import (
	"context"
	"testing"
)

func TestGetCloudflareDNSSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetCloudflareDNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetCloudflareDNSSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false on a fresh migration")
	}
}

func TestUpdateCloudflareDNSSettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateCloudflareDNSSettings(ctx, CloudflareDNSSettings{Enabled: true}); err != nil {
		t.Fatalf("UpdateCloudflareDNSSettings() error = %v", err)
	}

	got, err := db.GetCloudflareDNSSettings(ctx)
	if err != nil {
		t.Fatalf("GetCloudflareDNSSettings() error = %v", err)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true after update")
	}

	if err := db.UpdateCloudflareDNSSettings(ctx, CloudflareDNSSettings{Enabled: false}); err != nil {
		t.Fatalf("UpdateCloudflareDNSSettings() error = %v", err)
	}
	got, err = db.GetCloudflareDNSSettings(ctx)
	if err != nil {
		t.Fatalf("GetCloudflareDNSSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false after disabling")
	}
}

func TestCloudflareDNSSecretsKey_Stable(t *testing.T) {
	if got := CloudflareDNSSecretsKey(); got != CloudflareDNSSecretsKey() {
		t.Errorf("CloudflareDNSSecretsKey() is not stable across calls: %q vs %q", got, CloudflareDNSSecretsKey())
	}
}

func TestCloudflareDNSSecretsKey_DistinctFromTunnel(t *testing.T) {
	if CloudflareDNSSecretsKey() == CloudflareTunnelSecretsKey() {
		t.Errorf("CloudflareDNSSecretsKey() must not collide with CloudflareTunnelSecretsKey(): both %q", CloudflareDNSSecretsKey())
	}
}
