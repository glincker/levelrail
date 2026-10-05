package main

import (
	"log/slog"

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
		Maintainer: &backup.PITRMaintainer{
			Store:   db,
			Resolve: runner.ResolveDestination,
			Deleter: backup.S3Deleter{},
			Runtime: client,
			Logger:  logger,
		},
	}
}
