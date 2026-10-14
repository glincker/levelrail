package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploy/secretscan"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newSecretScanService(db *store.DB, logger *slog.Logger) *secretscan.Service {
	return secretscan.New(secretscan.NewSQLStore(db), secretscan.LimitsFromEnv(os.LookupEnv), logger)
}

// secretScanHook adapts the service to the deploy pipeline's hook.
type secretScanHook struct{ svc *secretscan.Service }

func (h secretScanHook) BeforeDeploy(ctx context.Context, app, attemptID, root string, env map[string]string) error {
	return h.svc.BeforeDeploy(ctx, secretscan.Input{App: app, AttemptID: attemptID, Root: root, Env: env})
}

func secretScanDeployOptions(db *store.DB, logger *slog.Logger) []deploy.Option {
	return []deploy.Option{deploy.WithSecretScan(secretScanHook{svc: newSecretScanService(db, logger)})}
}
