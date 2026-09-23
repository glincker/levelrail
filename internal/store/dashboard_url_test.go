package store

import (
	"context"
	"testing"
)

func TestDashboardURL_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetDashboardURL(ctx)
	if err != nil || got != "" {
		t.Fatalf("GetDashboardURL() on fresh db = %q, %v; want empty, nil", got, err)
	}

	for _, want := range []string{"https://dash.example.com", ""} {
		if err := db.SetDashboardURL(ctx, want); err != nil {
			t.Fatalf("SetDashboardURL(%q) error = %v", want, err)
		}
		got, err := db.GetDashboardURL(ctx)
		if err != nil {
			t.Fatalf("GetDashboardURL() error = %v", err)
		}
		if got != want {
			t.Errorf("GetDashboardURL() = %q, want %q", got, want)
		}
	}
}

func TestDashboardURL_SurvivesIngressSettingsUpdate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SetDashboardURL(ctx, "https://dash.example.com"); err != nil {
		t.Fatalf("SetDashboardURL() error = %v", err)
	}
	if err := db.UpdateIngressSettings(ctx, IngressSettings{PrimaryDomain: "dash.example.com"}); err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}
	got, err := db.GetDashboardURL(ctx)
	if err != nil {
		t.Fatalf("GetDashboardURL() error = %v", err)
	}
	if got != "https://dash.example.com" {
		t.Errorf("GetDashboardURL() = %q, want it untouched by UpdateIngressSettings", got)
	}
}
