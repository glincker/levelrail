package main

import (
	"context"
	"log/slog"
	"path/filepath"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// setupIngressSnapshot keeps the last applied route table on disk and serves it
// right away, so a restarted process answers before its first reconcile pass.
func setupIngressSnapshot(ctx context.Context, logger *slog.Logger, driver *ingressdriver.Driver, certs ingressdriver.CertStore, sm *secrets.Manager, edge ingressEdge) func() ingressdriver.SnapshotStatus {
	status := driver.SnapshotStatus
	key := sm.DeriveKey(ingressdriver.SnapshotKeyPurpose)
	if key == nil {
		logger.Warn("ingress route snapshot is off: no master key loaded to seal it")
		return status
	}
	store, err := ingressdriver.NewSnapshotStore(filepath.Join(dataDirFromEnv(), ingressdriver.SnapshotFilename), key)
	if err != nil {
		logger.Warn("ingress route snapshot is off", slog.String("error", err.Error()))
		return status
	}
	driver.SetSnapshot(store, edge.inherited.Fingerprint())
	ingressdriver.SetActiveCertStorage(ingressdriver.NewSQLiteStorage(certs, logger))
	if _, err := driver.RestoreSnapshot(ctx); err != nil {
		logger.Warn("ingress route snapshot not used, waiting for the first reconcile", slog.String("error", err.Error()))
	}
	return status
}
