package store

import (
	"context"
	"testing"
)

func TestGetRegistrySettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetRegistrySettings(context.Background())
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false on a fresh migration")
	}
	if got.Host != "" || got.Username != "" {
		t.Errorf("Host/Username = %q/%q, want empty on a fresh migration", got.Host, got.Username)
	}
}

func TestUpdateRegistrySettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := RegistrySettings{
		Enabled:   true,
		Host:      "registry.internal.example",
		Username:  "levelrail",
		CreatedAt: "2026-09-01T00:00:00Z",
	}
	if err := db.UpdateRegistrySettings(ctx, want); err != nil {
		t.Fatalf("UpdateRegistrySettings() error = %v", err)
	}

	got, err := db.GetRegistrySettings(ctx)
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if got != want {
		t.Errorf("GetRegistrySettings() = %+v, want %+v", got, want)
	}

	if err := db.UpdateRegistrySettings(ctx, RegistrySettings{Enabled: false}); err != nil {
		t.Fatalf("UpdateRegistrySettings() error = %v", err)
	}
	got, err = db.GetRegistrySettings(ctx)
	if err != nil {
		t.Fatalf("GetRegistrySettings() error = %v", err)
	}
	if got.Enabled || got.Host != "" || got.Username != "" {
		t.Errorf("GetRegistrySettings() = %+v, want all cleared after disabling", got)
	}
}

func TestRegistrySettingsSecretsKey_Stable(t *testing.T) {
	if got := RegistrySettingsSecretsKey(); got != RegistrySettingsSecretsKey() {
		t.Errorf("RegistrySettingsSecretsKey() is not stable across calls: %q vs %q", got, RegistrySettingsSecretsKey())
	}
}
