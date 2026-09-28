package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// setupLogArchive enables storage destinations and the log archive
// scheduler. It must run before the API server starts serving. Without a
// master key there is nowhere safe to keep bucket credentials, so it is a no-op.
func setupLogArchive(ctx context.Context, logger *slog.Logger, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, router *api.Router) {
	if secretsManager == nil {
		return
	}
	opts := objectstore.OptionsFromEnv(os.LookupEnv)
	resolver := &objectstore.Resolver{Store: db, Secrets: secretsManager}
	archiver := objectstore.NewArchiver(db, resolver, telemetryDB, logger, opts)
	buildCache := newBuildCache(logger, db, secretsManager)
	router.SetStorage(api.StorageDeps{
		Options: db, Archive: db, Clients: resolver, Archiver: archiver, ArchiveRoot: opts.Prefix,
		BuildCache: buildCache, BuildCacheSettings: db,
	})
	go archiver.Run(ctx)
}

func newBuildCache(logger *slog.Logger, db *store.DB, secretsManager *secrets.Manager) *objectstore.BuildCache {
	return &objectstore.BuildCache{
		Store:    db,
		Resolver: &objectstore.Resolver{Store: db, Secrets: secretsManager},
		Options:  objectstore.BuildCacheOptionsFromEnv(os.LookupEnv),
		Logger:   logger,
	}
}
