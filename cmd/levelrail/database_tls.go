package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// Generate once, persist forever, so a reconcile pass never rotates a running database's certificate.
// Returns nil, nil when mgr is nil: TLS needs the same secrets master key as credentialed engines.
func tlsMaterialFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.TLSMaterial, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.TLSCertEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing tls material for %q: %w", dbName, err)
	}
	if exists {
		current, err := mgr.Resolve(ctx, dbName, database.TLSCertEnvKey)
		if err != nil {
			return nil, fmt.Errorf("resolve tls certificate for %q: %w", dbName, err)
		}
		exists = !database.IsLegacyTLSCert([]byte(current))
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
