package store

import (
	"context"
	"testing"
)

func TestGetObservabilitySettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetObservabilitySettings(context.Background())
	if err != nil {
		t.Fatalf("GetObservabilitySettings() error = %v", err)
	}
	if got.ExternalDashboardURL != "" {
		t.Errorf("ExternalDashboardURL = %q, want empty on a fresh migration", got.ExternalDashboardURL)
	}
}

func TestUpdateObservabilitySettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	const url = "https://grafana.example.internal"
	if err := db.UpdateObservabilitySettings(ctx, ObservabilitySettings{ExternalDashboardURL: url}); err != nil {
		t.Fatalf("UpdateObservabilitySettings() error = %v", err)
	}

	got, err := db.GetObservabilitySettings(ctx)
	if err != nil {
		t.Fatalf("GetObservabilitySettings() error = %v", err)
	}
	if got.ExternalDashboardURL != url {
		t.Errorf("ExternalDashboardURL = %q, want %q", got.ExternalDashboardURL, url)
	}

	if err := db.UpdateObservabilitySettings(ctx, ObservabilitySettings{ExternalDashboardURL: ""}); err != nil {
		t.Fatalf("UpdateObservabilitySettings() error = %v", err)
	}
	got, err = db.GetObservabilitySettings(ctx)
	if err != nil {
		t.Fatalf("GetObservabilitySettings() error = %v", err)
	}
	if got.ExternalDashboardURL != "" {
		t.Errorf("ExternalDashboardURL = %q, want empty after clearing", got.ExternalDashboardURL)
	}
}
