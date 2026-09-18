package store

import (
	"context"
	"testing"
)

func TestGetVaultSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetVaultSettings(context.Background())
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false on a fresh migration")
	}
	if got.AuthMethod != VaultAuthMethodToken {
		t.Errorf("AuthMethod = %q, want %q", got.AuthMethod, VaultAuthMethodToken)
	}
	if got.MountPath != "secret" {
		t.Errorf("MountPath = %q, want \"secret\"", got.MountPath)
	}
}

func TestUpdateVaultSettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	in := VaultSettings{
		Enabled:    true,
		Address:    "https://vault.internal:8200",
		AuthMethod: VaultAuthMethodAppRole,
		Namespace:  "admin",
		RoleID:     "role-123",
		MountPath:  "kv",
	}
	if err := db.UpdateVaultSettings(ctx, in); err != nil {
		t.Fatalf("UpdateVaultSettings() error = %v", err)
	}

	got, err := db.GetVaultSettings(ctx)
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if got != in {
		t.Errorf("GetVaultSettings() = %+v, want %+v", got, in)
	}
}

func TestUpdateVaultSettings_EmptyMountPathDefaultsToSecret(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateVaultSettings(ctx, VaultSettings{Enabled: true, Address: "https://vault:8200", AuthMethod: VaultAuthMethodToken}); err != nil {
		t.Fatalf("UpdateVaultSettings() error = %v", err)
	}

	got, err := db.GetVaultSettings(ctx)
	if err != nil {
		t.Fatalf("GetVaultSettings() error = %v", err)
	}
	if got.MountPath != "secret" {
		t.Errorf("MountPath = %q, want \"secret\"", got.MountPath)
	}
}

func TestVaultConfigured(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	configured, err := db.VaultConfigured(ctx)
	if err != nil {
		t.Fatalf("VaultConfigured() error = %v", err)
	}
	if configured {
		t.Error("VaultConfigured() = true, want false on a fresh migration")
	}

	if err := db.UpdateVaultSettings(ctx, VaultSettings{Enabled: true, Address: "https://vault:8200", AuthMethod: VaultAuthMethodToken}); err != nil {
		t.Fatalf("UpdateVaultSettings() error = %v", err)
	}

	configured, err = db.VaultConfigured(ctx)
	if err != nil {
		t.Fatalf("VaultConfigured() error = %v", err)
	}
	if !configured {
		t.Error("VaultConfigured() = false, want true once enabled")
	}
}

func TestVaultSecretsKey_Stable(t *testing.T) {
	if got := VaultSecretsKey(); got != VaultSecretsKey() {
		t.Errorf("VaultSecretsKey() is not stable across calls: %q vs %q", got, VaultSecretsKey())
	}
}
