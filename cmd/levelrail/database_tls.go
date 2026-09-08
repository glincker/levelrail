package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// tlsMaterialFor generates and persists dbName's self-signed TLS
// certificate/key on first call, the same generate-once-persist-forever
// shape postgresCredentialsFor and its siblings already establish, and
// returns the same one on every subsequent call so a reconcile pass
// never rotates a running database's certificate out from under it.
//
// Returns (nil, nil), not an error, when mgr is nil, identical to
// postgresCredentialsFor's own reasoning: TLS is gated on the same
// secrets master key every credentialed database engine already
// requires, so a control plane without one reconciles every database
// exactly as it did before this feature existed.
func tlsMaterialFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.TLSMaterial, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.TLSCertEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing tls material for %q: %w", dbName, err)
	}
	if !exists {
		certPEM, keyPEM, err := database.GenerateSelfSignedCert(database.ContainerName(dbName))
		if err != nil {
			return nil, fmt.Errorf("generate tls material for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.TLSCertEnvKey, string(certPEM)); err != nil {
			return nil, fmt.Errorf("store tls certificate for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.TLSKeyEnvKey, string(keyPEM)); err != nil {
			return nil, fmt.Errorf("store tls key for %q: %w", dbName, err)
		}
	}

	certPEM, err := mgr.Resolve(ctx, dbName, database.TLSCertEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve tls certificate for %q: %w", dbName, err)
	}
	keyPEM, err := mgr.Resolve(ctx, dbName, database.TLSKeyEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve tls key for %q: %w", dbName, err)
	}
	if certPEM == "" || keyPEM == "" {
		return nil, errors.New("resolved tls material is empty")
	}

	return &database.TLSMaterial{CertPEM: []byte(certPEM), KeyPEM: []byte(keyPEM)}, nil
}
