package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newBaseBackupRunner builds the physical base backup runner shared by the
// manual trigger and the scheduler, with retention so a PITR-enabled
// database's WAL archive and base backups stay bounded.
func newBaseBackupRunner(db *store.DB, secretsManager *secrets.Manager, client *docker.Client, runner *backup.Runner, logger *slog.Logger) *backup.BaseBackupRunner {
	return &backup.BaseBackupRunner{
		Store:        db,
		Secrets:      secretsManager,
		BaseBackuper: &backup.ContainerBaseBackuper{Runtime: client},
		Uploader:     backup.S3Uploader{},
		Runtime:      client,
		WAL:          &backup.WALShipper{Runtime: client, Uploader: backup.S3Uploader{}, Lister: backup.S3Lister{}, Logger: logger},
		Maintainer: &backup.PITRMaintainer{
			Store:   db,
			Resolve: runner.ResolveDestination,
			Deleter: backup.S3Deleter{},
			Lister:  backup.S3Lister{},
			Runtime: client,
			Logger:  logger,
		},
	}
}

// databaseWALArchiveVolume mirrors internal/reconcile/database's volume name.
func databaseWALArchiveVolume(databaseName string) string {
	return "db-" + databaseName + "-wal-archive"
}

// newWALShipScheduler builds the periodic WAL shipper for local PITR-enabled
// Postgres databases.
func newWALShipScheduler(db *store.DB, client *docker.Client, runner *backup.Runner, isLocal func(nodeID string) bool, logger *slog.Logger) *backup.WALShipScheduler {
	return &backup.WALShipScheduler{
		Store:         db,
		Shipper:       &backup.WALShipper{Runtime: client, Uploader: backup.S3Uploader{}, Lister: backup.S3Lister{}, Logger: logger},
		Resolve:       runner.ResolveDestination,
		ContainerName: func(name string) string { return "db-" + name },
		IsLocal:       isLocal,
		Logger:        logger,
	}
}

// defaultWALShipInterval bounds how much WAL is lost if the database host's
// disk dies: roughly one interval of writes.
const defaultWALShipInterval = time.Minute

// walShipInterval reads APP_PITR_WAL_SHIP_INTERVAL; 0 disables periodic shipping.
func walShipInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_PITR_WAL_SHIP_INTERVAL")
	if raw == "" {
		return defaultWALShipInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid APP_PITR_WAL_SHIP_INTERVAL, using the default", slog.String("value", raw))
		return defaultWALShipInterval
	}
	return d
}

// newMajorUpgradeRunner builds the guarded Postgres major upgrade runner and
// settles any upgrade a previous run of the control plane left half done.
func newMajorUpgradeRunner(db *store.DB, client *docker.Client, nudge func(), logger *slog.Logger) *backup.MajorUpgradeRunner {
	r := &backup.MajorUpgradeRunner{
		Store:    db,
		Runtime:  client,
		Dumper:   &backup.ContainerDumper{Runtime: client},
		Restorer: &backup.ContainerRestorer{Runtime: client},
		Nudge:    nudge,
		Logger:   logger,
		Wait:     durationFromEnv("APP_MAJOR_UPGRADE_WAIT", 0, logger),
	}
	time.AfterFunc(15*time.Second, func() { r.RecoverInterrupted(context.Background()) })
	return r
}

// durationFromEnv parses a Go duration from env key, returning def when it is
// unset or invalid.
func durationFromEnv(key string, def time.Duration, logger *slog.Logger) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logger.Warn("invalid duration in environment, using the default", slog.String("key", key), slog.String("value", raw))
		return def
	}
	return d
}
