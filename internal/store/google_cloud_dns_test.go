package store

import (
	"context"
	"testing"
)

func TestGoogleCloudDNSSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetGoogleCloudDNSSettings(ctx)
	if err != nil || got != (GoogleCloudDNSSettings{}) {
		t.Fatalf("seeded default = %+v, %v", got, err)
	}

	want := GoogleCloudDNSSettings{Enabled: true, ProjectID: "proj-1", ClientEmail: "sa@proj-1.iam.gserviceaccount.com"}
	if err := db.UpdateGoogleCloudDNSSettings(ctx, want); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = db.GetGoogleCloudDNSSettings(ctx)
	if err != nil || got != want {
		t.Fatalf("round trip = %+v, %v; want %+v", got, err, want)
	}
	if GoogleCloudDNSSecretsKey() == Route53DNSSecretsKey() || GoogleCloudDNSSecretsKey() == CloudflareDNSSecretsKey() {
		t.Fatal("secrets key collides with another DNS provider")
	}
}
