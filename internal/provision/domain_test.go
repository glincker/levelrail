package provision

import "testing"

func TestFailureDomain(t *testing.T) {
	tests := []struct {
		name, provider, id, region, zone string
	}{
		{"gcp zone", "gcp", "us-central1-a", "us-central1", "us-central1-a"},
		{"gcp no dash", "gcp", "global", "global", ""},
		{"hetzner", "hetzner", "fsn1", "fsn1", "fsn1"},
		{"digitalocean", "digitalocean", "nyc3", "nyc3", "nyc3"},
		{"aws", "aws", "us-east-1", "us-east-1", ""},
		{"azure", "azure", "westeurope", "westeurope", ""},
		{"unknown", "other", "x", "x", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			region, zone := FailureDomain(tc.provider, tc.id)
			if region != tc.region || zone != tc.zone {
				t.Fatalf("FailureDomain(%q, %q) = %q, %q; want %q, %q", tc.provider, tc.id, region, zone, tc.region, tc.zone)
			}
		})
	}
}
