package main

import (
	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newComposeServiceBackupRunner shares the database runner's destination, encryption and upload path.
func newComposeServiceBackupRunner(db *store.DB, client *docker.Client, registry *agent.Registry, runner *backup.Runner) *backup.ComposeServiceRunner {
	nodeRuntime := func(nodeID string) (docker.Runtime, error) {
		return resolveNodeTransport(client, registry, nodeID)
	}
	return &backup.ComposeServiceRunner{
		Runner: runner,
		Runs:   db,
		Containers: &backup.DesiredComposeResolver{
			Services: db,
			Runtimes: nodeRuntime,
			Owns:     application.OwnsContainer,
		},
		Dumper: &backup.ComposeServiceDumper{Runtimes: nodeRuntime},
	}
}

// composeBackupOption stays a no-op without a master key, like the other backup options.
// The restore side reuses the database restore runner's downloader, restorer and decrypt identities.
func composeBackupOption(db *store.DB, client *docker.Client, registry *agent.Registry, runner *backup.Runner, restore *backup.RestoreRunner) api.Option {
	if runner == nil {
		return func(*api.Router) {}
	}
	dumper := newComposeServiceBackupRunner(db, client, registry, runner)
	nodeRuntime := func(nodeID string) (docker.Runtime, error) {
		return resolveNodeTransport(client, registry, nodeID)
	}
	restorer := &backup.ComposeServiceRestorer{
		Safety: dumper, Destinations: runner, Containers: dumper.Containers,
		Downloader: restore.Downloader, Restorer: &backup.ComposeContainerRestorer{Runtimes: nodeRuntime}, Identities: restore.Identities,
	}
	return func(rt *api.Router) {
		api.WithComposeDBBackupRunner(dumper)(rt)
		api.WithComposeDBRestoreRunner(restorer)(rt)
	}
}
