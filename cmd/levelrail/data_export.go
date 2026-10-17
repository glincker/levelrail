package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"

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

// Without a master key, destinations needing a header or signing key cannot be saved.
// Unauthenticated destinations and the scrape endpoint still work.
func setupDataExport(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, logs *telemetry.LogBroadcaster, router *api.Router) {
	deps := dataexport.Deps{
		Store:    db,
		Platform: db,
		Logs:     logs,
		Config:   dataexport.ConfigFromEnv(os.LookupEnv),
		Brand:    b.ShortName,
		Logger:   logger,
		AppCount: func(ctx context.Context) (int, error) {
			svcs, err := db.ListDesiredServices(ctx)
			return len(svcs), err
		},
	}
	deps.SpoolDir = exportSpoolDir(os.LookupEnv)
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

// exportSpoolDir is where log destinations keep their on-disk backlog:
// APP_EXPORT_SPOOL_DIR, else a folder under the data directory. Setting
// APP_EXPORT_SPOOL=false keeps backlogs in memory only.
func exportSpoolDir(lookup func(string) (string, bool)) string {
	if v, ok := lookup("APP_EXPORT_SPOOL"); ok && (v == "false" || v == "0") {
		return ""
	}
	if v, ok := lookup("APP_EXPORT_SPOOL_DIR"); ok && v != "" {
		return v
	}
	dir, ok := lookup("APP_DATA_DIR")
	if !ok || dir == "" {
		dir = defaultDataDir
	}
	return filepath.Join(dir, "export-spool")
}
