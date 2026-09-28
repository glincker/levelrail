package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
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
	if err := agent.NewIdentityFile(path).Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
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

func issuedIdentity(t *testing.T, ca *agent.CA, validFor time.Duration) *agent.Identity {
	t.Helper()
	keyPEM, csr, err := agent.NewKeyAndCSR("node-1")
	if err != nil {
		t.Fatalf("NewKeyAndCSR() error = %v", err)
	}
	issued, err := ca.SignClientCSR("node-1", csr, validFor, time.Now())
	if err != nil {
		t.Fatalf("SignClientCSR() error = %v", err)
	}
	return &agent.Identity{NodeID: "node-1", ClientCertPEM: issued.PEM, ClientKeyPEM: keyPEM, CACertPEM: ca.CertPEM()}
}

func TestAdoptIdentityFromDisk(t *testing.T) {
	ca, err := agent.GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	expired := issuedIdentity(t, ca, -time.Minute)
	fresh := issuedIdentity(t, ca, time.Hour)

	tests := []struct {
		name   string
		onDisk *agent.Identity
		want   bool
	}{
		{"re-enrolled identity on disk is adopted", fresh, true},
		{"same identity is not adopted again", expired, false},
		{"expired identity on disk is ignored", issuedIdentity(t, ca, -time.Hour), false},
		{"no identity file", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := agent.NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
			if tt.onDisk != nil {
				if err := file.Save(tt.onDisk); err != nil {
					t.Fatalf("Save() error = %v", err)
				}
			}
			holder := agent.NewIdentityHolder(expired)
			if got := adoptIdentityFromDisk(holder, file, testLogger()); got != tt.want {
				t.Fatalf("adoptIdentityFromDisk() = %v, want %v", got, tt.want)
			}
			if tt.want && holder.Current() == expired {
				t.Error("holder was not switched to the identity on disk")
			}
		})
	}
}

func TestRunReenroll_RequiresToken(t *testing.T) {
	t.Setenv("APP_REENROLL_TOKEN", "")
	if err := runReenroll(context.Background(), "127.0.0.1:1", filepath.Join(t.TempDir(), "identity.json"), testLogger()); err == nil {
		t.Fatal("runReenroll() error = nil, want an error without APP_REENROLL_TOKEN")
	}
}

func TestFloatFromEnv(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want float64
	}{{"0.5", 0.5}, {"", 0}, {"nope", 0}} {
		t.Setenv("APP_AGENT_CERT_RENEW_FRACTION", tt.raw)
		if got := floatFromEnv("APP_AGENT_CERT_RENEW_FRACTION"); got != tt.want {
			t.Errorf("floatFromEnv(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}
