package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/dbupgrade"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startDatabaseUpgrades wires the upgrade advisor, policy API and runner,
// then starts the controller and the optional registry tag refresh.
// Backups need the master key; without it every run fails at its backup step.
func startDatabaseUpgrades(ctx context.Context, logger *slog.Logger, db *store.DB, secretsManager *secrets.Manager,
	client *docker.Client, nudge func(), backupRunner *backup.Runner, verifyRunner *backup.VerifyRunner,
	alertingDB *alerting.DB, dispatcher *alerting.DeployDispatcher, isLocal func(nodeID string) bool, apiRouter *api.Router) {
	catalog, err := dbupgrade.LoadCatalog()
	if err != nil {
		logger.Error("database upgrades disabled: catalog failed to load", slog.String("error", err.Error()))
		return
	}
	rt := &dbupgrade.DockerRuntime{
		Store: db, Docker: client, CopyVolume: backup.CopyVolume, WipeVolume: backup.WipeVolume, Nudge: nudge,
		StopWait: dbupgrade.DurationFromEnv(dbupgrade.EnvHealthTimeout, dbupgrade.DefaultHealthTimeout, logger),
	}
	if backupRunner != nil {
		rt.Backups = backupRunner
	}
	if verifyRunner != nil {
		rt.Verifier = verifyRunner
	}
	if secretsManager != nil {
		rt.Restorer = &backup.RestoreRunner{
			Store: db, Secrets: secretsManager, Downloader: backup.S3Downloader{}, Restorer: &backup.ContainerRestorer{Runtime: client},
		}
	}
	advisor := dbupgrade.Advisor{Catalog: catalog, EOLWarn: dbupgrade.DurationFromEnv(dbupgrade.EnvEOLWarn, dbupgrade.DefaultEOLWarn, logger)}
	runner := &dbupgrade.Runner{
		Store: db, Runtime: rt, Catalog: catalog, Logger: logger,
		Notifier:      dbupgrade.ChannelNotifier{Channels: alertingDB, Sender: dispatcher},
		HealthTimeout: dbupgrade.DurationFromEnv(dbupgrade.EnvHealthTimeout, dbupgrade.DefaultHealthTimeout, logger),
	}
	manager := &dbupgrade.Manager{
		Store: db, Runner: runner, Advisor: advisor, Logger: logger,
		MinWindowRemaining: dbupgrade.DurationFromEnv(dbupgrade.EnvMinWindowRemaining, dbupgrade.DefaultMinWindowRemaining, logger),
		Unreachable: func(d store.DesiredDatabase) string {
			if isLocal(d.NodeID) {
				return ""
			}
			return "the database runs on node " + d.NodeID + "; upgrades run only for databases on the control plane node"
		},
	}
	apiRouter.SetDatabaseUpgrader(manager)
	go manager.Run(ctx, dbupgrade.DurationFromEnv(dbupgrade.EnvInterval, dbupgrade.DefaultInterval, logger))

	refresher := &dbupgrade.Refresher{Catalog: catalog, Lister: dbupgrade.DockerHubLister{}, Logger: logger}
	go refresher.Run(ctx, dbupgrade.RefreshIntervalFromEnv(logger))
}
