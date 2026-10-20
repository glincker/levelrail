package store

import (
	"context"
	"testing"
)

func TestGetRoute53DNSSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetRoute53DNSSettings(context.Background())
	if err != nil {
		t.Fatalf("GetRoute53DNSSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false on a fresh migration")
	}
	if got.Region != "" || got.HostedZoneID != "" {
		t.Errorf("Region/HostedZoneID = %q/%q, want both empty on a fresh migration", got.Region, got.HostedZoneID)
	}
}

func TestUpdateRoute53DNSSettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := Route53DNSSettings{Enabled: true, Region: "us-east-1", HostedZoneID: "Z123456"}
	if err := db.UpdateRoute53DNSSettings(ctx, want); err != nil {
		t.Fatalf("UpdateRoute53DNSSettings() error = %v", err)
	}

	got, err := db.GetRoute53DNSSettings(ctx)
	if err != nil {
		t.Fatalf("GetRoute53DNSSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if err := db.UpdateRoute53DNSSettings(ctx, Route53DNSSettings{}); err != nil {
		t.Fatalf("UpdateRoute53DNSSettings() error = %v", err)
	}
	got, err = db.GetRoute53DNSSettings(ctx)
	if err != nil {
		t.Fatalf("GetRoute53DNSSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false after disabling")
	}
}

func TestRoute53DNSSecretsKey_Stable(t *testing.T) {
	if got := Route53DNSSecretsKey(); got != Route53DNSSecretsKey() {
		t.Errorf("Route53DNSSecretsKey() is not stable across calls: %q vs %q", got, Route53DNSSecretsKey())
	}
}

func TestRoute53DNSSecretsKey_DistinctFromCloudflareDNS(t *testing.T) {
	if Route53DNSSecretsKey() == CloudflareDNSSecretsKey() {
		t.Errorf("Route53DNSSecretsKey() must not collide with CloudflareDNSSecretsKey(): both %q", Route53DNSSecretsKey())
	}
}
