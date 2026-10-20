package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
)

// gracefulStopper is the subset of *grpc.Server stopAgentGRPCServer
// needs, narrowed so a test can exercise the timeout/fallback path
// without a real listener and a real stuck stream.
type gracefulStopper interface {
	GracefulStop()
	Stop()
}

// Falls back to hard Stop so a live agent Session stream cannot stall process exit.
func stopAgentGRPCServer(server gracefulStopper, logger *slog.Logger, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		logger.Warn("agent grpc server did not drain in time, forcing stop",
			slog.Duration("timeout", timeout))
		server.Stop()
		<-done
	}
}

func openStore(ctx context.Context) (*store.DB, error) {
	return openStoreObserved(ctx, nil)
}

// openStoreObserved is openStore that reports pre-migration facts into obs.
func openStoreObserved(ctx context.Context, obs *upgradehistory.Observed) (*store.DB, error) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	// dataDir is operator-supplied at process startup (env var or the
	// fixed default), not attacker-controlled request input, so gosec's
	// path-traversal concern doesn't apply here.
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	return store.Open(ctx, filepath.Join(dataDir, storeFilename), store.WithPreMigrateHook(observedPreMigrateSnapshotHook(dataDir, obs)))
}

// Separate SQLite file so its collection-tick writes never contend with levelrail.db's WAL.
func openTelemetryStore(ctx context.Context) (*telemetry.DB, error) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	return telemetry.Open(ctx, filepath.Join(dataDir, "telemetry.db"))
}

// Separate SQLite file for the same write-isolation reason as telemetry.db.
func openAlertingStore(ctx context.Context) (*alerting.DB, error) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	return alerting.Open(ctx, filepath.Join(dataDir, "alerting.db"))
}
