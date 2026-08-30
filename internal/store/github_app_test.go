package store

import (
	"context"
	"errors"
	"testing"
)

func newTestGitHubAppConnection() GitHubAppConnection {
	return GitHubAppConnection{
		AppID:       123456,
		ClientID:    "Iv1.testclientid",
		InstanceURL: "https://github.com",
		CreatedAt:   "2026-08-14T00:00:00Z",
	}
}

func TestSaveAndGetGitHubAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestGitHubAppConnection()
	if err := db.SaveGitHubAppConnection(ctx, want); err != nil {
		t.Fatalf("SaveGitHubAppConnection() error = %v", err)
	}

	got, err := db.GetGitHubAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if got.AppID != want.AppID || got.ClientID != want.ClientID || got.InstanceURL != want.InstanceURL || got.CreatedAt != want.CreatedAt {
		t.Errorf("GetGitHubAppConnection() = %+v, want %+v", got, want)
	}
	if got.InstallationID != nil {
		t.Errorf("GetGitHubAppConnection().InstallationID = %v, want nil (not yet installed)", *got.InstallationID)
	}
	if got.AccountLogin != nil {
		t.Errorf("GetGitHubAppConnection().AccountLogin = %v, want nil (not yet installed)", *got.AccountLogin)
	}
}

// TestSaveAndGetGitHubAppConnection_GHEInstanceURL proves a GitHub
// Enterprise Server instance URL round-trips distinctly from the
// github.com default (migrations/0061).
func TestSaveAndGetGitHubAppConnection_GHEInstanceURL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestGitHubAppConnection()
	want.InstanceURL = "https://ghe.example.com"
	if err := db.SaveGitHubAppConnection(ctx, want); err != nil {
		t.Fatalf("SaveGitHubAppConnection() error = %v", err)
	}

	got, err := db.GetGitHubAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if got.InstanceURL != "https://ghe.example.com" {
		t.Errorf("GetGitHubAppConnection().InstanceURL = %q, want the GHE instance URL", got.InstanceURL)
	}
}

func TestGetGitHubAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetGitHubAppConnection(ctx)
	if !errors.Is(err, ErrGitHubAppConnectionNotFound) {
		t.Fatalf("GetGitHubAppConnection() error = %v, want ErrGitHubAppConnectionNotFound", err)
	}
}

func TestSaveGitHubAppConnection_ReplacesExisting(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestGitHubAppConnection()
	if err := db.SaveGitHubAppConnection(ctx, first); err != nil {
		t.Fatalf("SaveGitHubAppConnection(first) error = %v", err)
	}

	second := newTestGitHubAppConnection()
	second.AppID = 999999
	second.ClientID = "Iv1.secondclientid"
	if err := db.SaveGitHubAppConnection(ctx, second); err != nil {
		t.Fatalf("SaveGitHubAppConnection(second) error = %v", err)
	}

	got, err := db.GetGitHubAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if got.AppID != second.AppID || got.ClientID != second.ClientID {
		t.Errorf("GetGitHubAppConnection() = %+v, want the second (replacing) connection %+v", got, second)
	}
}

func TestUpdateGitHubAppInstallation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	conn := newTestGitHubAppConnection()
	if err := db.SaveGitHubAppConnection(ctx, conn); err != nil {
		t.Fatalf("SaveGitHubAppConnection() error = %v", err)
	}

	if err := db.UpdateGitHubAppInstallation(ctx, 42, "octocat"); err != nil {
		t.Fatalf("UpdateGitHubAppInstallation() error = %v", err)
	}

	got, err := db.GetGitHubAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if got.InstallationID == nil || *got.InstallationID != 42 {
		t.Errorf("GetGitHubAppConnection().InstallationID = %v, want 42", got.InstallationID)
	}
	if got.AccountLogin == nil || *got.AccountLogin != "octocat" {
		t.Errorf("GetGitHubAppConnection().AccountLogin = %v, want \"octocat\"", got.AccountLogin)
	}
}

func TestUpdateGitHubAppInstallation_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateGitHubAppInstallation(ctx, 42, "octocat")
	if !errors.Is(err, ErrGitHubAppConnectionNotFound) {
		t.Fatalf("UpdateGitHubAppInstallation() error = %v, want ErrGitHubAppConnectionNotFound", err)
	}
}

func TestDeleteGitHubAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	conn := newTestGitHubAppConnection()
	if err := db.SaveGitHubAppConnection(ctx, conn); err != nil {
		t.Fatalf("SaveGitHubAppConnection() error = %v", err)
	}

	if err := db.DeleteGitHubAppConnection(ctx); err != nil {
		t.Fatalf("DeleteGitHubAppConnection() error = %v", err)
	}

	_, err := db.GetGitHubAppConnection(ctx)
	if !errors.Is(err, ErrGitHubAppConnectionNotFound) {
		t.Fatalf("GetGitHubAppConnection() after delete error = %v, want ErrGitHubAppConnectionNotFound", err)
	}
}

func TestDeleteGitHubAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteGitHubAppConnection(ctx)
	if !errors.Is(err, ErrGitHubAppConnectionNotFound) {
		t.Fatalf("DeleteGitHubAppConnection() error = %v, want ErrGitHubAppConnectionNotFound", err)
	}
}

func TestGitHubAppSecretsKey_CannotCollideWithServiceName(t *testing.T) {
	key := GitHubAppSecretsKey()
	if key == "" {
		t.Fatal("GitHubAppSecretsKey() returned empty string")
	}
	// The whole point of this synthetic key: a "/" can never appear in a
	// real desired_services row name (Docker-container/DNS-label-safe
	// tokens only), so this key is structurally distinct from every real
	// service name, the same guarantee BackupTargetSecretsKey relies on.
	if !containsSlash(key) {
		t.Fatalf("GitHubAppSecretsKey() = %q, want a %q-containing sentinel key", key, "/")
	}
}

func containsSlash(s string) bool {
	for _, r := range s {
		if r == '/' {
			return true
		}
	}
	return false
}
