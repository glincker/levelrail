package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/extdb"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startExternalDatabaseMonitor probes every external database on an
// interval. Each probe is a read-only connection test from a short-lived
// helper container; APP_EXTERNAL_DB_PROBE_INTERVAL=0 turns it off.
func startExternalDatabaseMonitor(ctx context.Context, logger *slog.Logger, db *store.DB, secretsManager *secrets.Manager, local docker.Runtime, registry *agent.Registry) {
	interval := extdb.IntervalFromEnv()
	if interval <= 0 {
		logger.Info("external database health probes disabled (APP_EXTERNAL_DB_PROBE_INTERVAL=0)")
		return
	}
	m := &extdb.Monitor{
		Store:   db,
		Runtime: func(nodeID string) (docker.Runtime, error) { return resolveNodeTransport(local, registry, nodeID) },
		Logger:  logger,
	}
	if secretsManager != nil {
		m.Secrets = secretsManager
	}
	go m.Run(ctx, interval)
}
