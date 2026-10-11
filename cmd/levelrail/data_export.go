package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// storagePutter writes export objects to a connected storage destination.
type storagePutter struct{ resolver *objectstore.Resolver }

func (p storagePutter) PutObject(ctx context.Context, targetID, key string, body []byte, contentType string) error {
	client, _, err := p.resolver.Client(ctx, targetID)
	if err != nil {
		return err
	}
	return client.Put(ctx, key, bytes.NewReader(body), contentType, "")
}

// setupDataExport starts the opt-in log, audit and metric exporters and
// enables the export routes and the /metrics scrape endpoint. Without a
// master key, destinations that need a header or signing key cannot be
// saved, but unauthenticated ones and the scrape endpoint still work.
func setupDataExport(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, logs *telemetry.LogBroadcaster, router *api.Router) {
	deps := dataexport.Deps{
		Store:  db,
		Logs:   logs,
		Config: dataexport.ConfigFromEnv(os.LookupEnv),
		Brand:  b.ShortName,
		Logger: logger,
		AppCount: func(ctx context.Context) (int, error) {
			svcs, err := db.ListDesiredServices(ctx)
			return len(svcs), err
		},
	}
	if telemetryDB != nil {
		deps.Metrics = telemetryDB
	}
	if secretsManager != nil {
		deps.Secrets = secretsManager
		deps.Objects = storagePutter{resolver: &objectstore.Resolver{Store: db, Secrets: secretsManager}}
	}
	svc := dataexport.NewService(deps)
	router.SetDataExport(svc)
	go svc.Run(ctx)
}
