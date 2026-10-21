package migrationmode

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/agent/pinned"
)

// ErrNoPin means the install command carried no CA fingerprint. Migration
// enrollment never falls back to trust on first use.
var ErrNoPin = errors.New("migrationmode: a CA fingerprint is required")

// Identity is the short-lived credential persisted between restarts.
type Identity struct {
	SessionID     string    `json:"session_id"`
	ClientCertPEM []byte    `json:"client_cert_pem"`
	ClientKeyPEM  []byte    `json:"client_key_pem"`
	CACertPEM     []byte    `json:"ca_cert_pem"`
	NotAfter      time.Time `json:"not_after"`
	ExpiresAt     time.Time `json:"session_expires_at"`
}

// Enroll exchanges a single-use token for an Identity. The control plane CA
// must match caFingerprint before the token is sent.
func Enroll(ctx context.Context, addr, token, caFingerprint, version string) (*Identity, error) {
	fp := pinned.Normalize(caFingerprint)
	if fp == "" {
		return nil, ErrNoPin
	}
	keyPEM, csr, err := pinned.NewKeyAndCSR("migration-source")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(pinned.EnrollTLSConfig(fp))))
	if err != nil {
		return nil, fmt.Errorf("migrationmode: dial %q: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	resp, err := migrationpb.NewMigrationServiceClient(conn).Enroll(ctx, &migrationpb.EnrollRequest{
		JoinToken: token, CsrDer: csr, Version: version, Os: runtime.GOOS, Arch: runtime.GOARCH, CaPinState: "pinned"})
	if err != nil {
		return nil, fmt.Errorf("migrationmode: enroll: %w", err)
	}
	if err := pinned.CheckCA(resp.GetCaCertPem(), fp); err != nil {
		return nil, fmt.Errorf("migrationmode: enroll: %w", err)
	}
	if _, err := tls.X509KeyPair(resp.GetClientCertPem(), keyPEM); err != nil {
		return nil, fmt.Errorf("migrationmode: issued certificate does not match our key: %w", err)
	}
	return &Identity{SessionID: resp.GetSessionId(), ClientCertPEM: resp.GetClientCertPem(), ClientKeyPEM: keyPEM,
		CACertPEM: resp.GetCaCertPem(), NotAfter: resp.GetNotAfter().AsTime(), ExpiresAt: resp.GetSessionExpiresAt().AsTime()}, nil
}

// SaveIdentity writes id to path with owner-only permissions, atomically.
func SaveIdentity(path string, id *Identity) error {
	raw, err := json.Marshal(id)
	if err != nil {
		return fmt.Errorf("migrationmode: encode identity: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("migrationmode: create identity dir: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil { //nolint:gosec // 0700 is the owner-only mode this fix enforces
		return fmt.Errorf("migrationmode: restrict identity dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("migrationmode: write identity: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("migrationmode: replace identity: %w", err)
	}
	return nil
}

// LoadIdentity reads an identity file, or returns os.ErrNotExist.
func LoadIdentity(path string) (*Identity, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is the operator-chosen state dir
	if err != nil {
		return nil, fmt.Errorf("migrationmode: read identity: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, fmt.Errorf("migrationmode: decode identity: %w", err)
	}
	return &id, nil
}

// Connect opens the mTLS Session stream and serves it until it ends.
func Connect(ctx context.Context, addr string, id *Identity, h *Handler, version string, heartbeat time.Duration) error {
	cfg, err := pinned.SessionTLSConfig(id.CACertPEM, id.ClientCertPEM, id.ClientKeyPEM)
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: heartbeat, Timeout: heartbeat, PermitWithoutStream: true}))
	if err != nil {
		return fmt.Errorf("migrationmode: dial %q: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := migrationpb.NewMigrationServiceClient(conn).Session(sctx)
	if err != nil {
		return fmt.Errorf("migrationmode: open session: %w", err)
	}
	return Serve(sctx, stream, h, version, runtime.GOOS, runtime.GOARCH, heartbeat)
}
