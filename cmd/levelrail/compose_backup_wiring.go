package main

import (
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newComposeServiceBackupRunner shares the database runner's destination, encryption and upload path.
func newComposeServiceBackupRunner(db *store.DB, client *docker.Client, runner *backup.Runner) *backup.ComposeServiceRunner {
	return &backup.ComposeServiceRunner{
		Runner: runner,
		Runs:   db,
		Containers: &backup.DesiredComposeResolver{
			Services: db,
			Runtime:  client,
			Owns:     application.OwnsContainer,
		},
		Dumper: &backup.ComposeServiceDumper{Runtime: client},
	}
}

// composeBackupOption stays a no-op without a master key, like the other backup options.
func composeBackupOption(db *store.DB, client *docker.Client, runner *backup.Runner) api.Option {
	if runner == nil {
		return func(*api.Router) {}
	}
	return api.WithComposeDBBackupRunner(newComposeServiceBackupRunner(db, client, runner))
}
