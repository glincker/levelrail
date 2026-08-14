package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestSaveAndLoadIdentity_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	want := &agent.Identity{
		NodeID:        "node-1",
		ClientCertPEM: []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n"),
		ClientKeyPEM:  []byte("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n"),
		CACertPEM:     []byte("-----BEGIN CERTIFICATE-----\nfakeca\n-----END CERTIFICATE-----\n"),
	}

	if err := saveIdentity(path, want); err != nil {
		t.Fatalf("saveIdentity() error = %v", err)
	}

	got, err := loadIdentity(path)
	if err != nil {
		t.Fatalf("loadIdentity() error = %v", err)
	}
	if got.NodeID != want.NodeID {
		t.Errorf("NodeID = %q, want %q", got.NodeID, want.NodeID)
	}
	if string(got.ClientCertPEM) != string(want.ClientCertPEM) {
		t.Errorf("ClientCertPEM mismatch")
	}
	if string(got.ClientKeyPEM) != string(want.ClientKeyPEM) {
		t.Errorf("ClientKeyPEM mismatch")
	}
	if string(got.CACertPEM) != string(want.CACertPEM) {
		t.Errorf("CACertPEM mismatch")
	}
}

func TestSaveIdentity_FilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := saveIdentity(path, &agent.Identity{NodeID: "node-1"}); err != nil {
		t.Fatalf("saveIdentity() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600 (contains a private key)", info.Mode().Perm())
	}
}

func TestLoadIdentity_NotFound(t *testing.T) {
	_, err := loadIdentity(filepath.Join(t.TempDir(), "nonexistent.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("loadIdentity() error = %v, want os.ErrNotExist", err)
	}
}

func TestLoadIdentity_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := loadIdentity(path); err == nil {
		t.Error("loadIdentity() error = nil, want an error for invalid JSON")
	}
}

func TestIdentityFilePath_Default(t *testing.T) {
	t.Setenv("APP_AGENT_IDENTITY_FILE", "")
	if got := identityFilePath(); got != defaultIdentityFile {
		t.Errorf("identityFilePath() = %q, want %q", got, defaultIdentityFile)
	}
}

func TestIdentityFilePath_EnvOverride(t *testing.T) {
	t.Setenv("APP_AGENT_IDENTITY_FILE", "/custom/path.json")
	if got := identityFilePath(); got != "/custom/path.json" {
		t.Errorf("identityFilePath() = %q, want /custom/path.json", got)
	}
}

func TestLoadOrEnroll_LoadsExistingIdentity_NeverCallsEnroll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	want := &agent.Identity{NodeID: "node-1", ClientCertPEM: []byte("cert"), ClientKeyPEM: []byte("key"), CACertPEM: []byte("ca")}
	if err := saveIdentity(path, want); err != nil {
		t.Fatalf("saveIdentity() error = %v", err)
	}

	// addr is deliberately unreachable: if loadOrEnroll tried to dial
	// it, this test would hang or fail on a connection error instead of
	// returning quickly, proving the existing identity was used instead.
	got, err := loadOrEnroll(context.Background(), "127.0.0.1:1", path, testLogger())
	if err != nil {
		t.Fatalf("loadOrEnroll() error = %v", err)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1", got.NodeID)
	}
}

func TestLoadOrEnroll_NoIdentityNoToken_Errors(t *testing.T) {
	t.Setenv("APP_JOIN_TOKEN", "")
	path := filepath.Join(t.TempDir(), "identity.json")

	_, err := loadOrEnroll(context.Background(), "127.0.0.1:1", path, testLogger())
	if err == nil {
		t.Fatal("loadOrEnroll() error = nil, want an error when no identity exists and no join token is set")
	}
}
