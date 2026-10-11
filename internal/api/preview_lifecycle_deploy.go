package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const previewDatabaseSourceKey = "preview"

// finishPreviewExposure runs the steps that make a freshly deployed preview
// reachable and safe: DNS record, search visibility, basic auth gate and idle
// sleep. Each failure becomes a note on the preview, never a failed deploy.
func (rt *Router) finishPreviewExposure(ctx context.Context, appName, previewName, domain string, s store.PreviewAppSettings) []string {
	var notes []string
	if domain != "" {
		res := rt.previewDNS(ctx, previewName, domain)
		if res.DNS == dnsResultError {
			rt.logger.Warn("api: preview dns record failed", slog.String("domain", domain), slog.String("message", res.Message))
			notes = append(notes, "the DNS record could not be created: "+res.Message)
		}
	}
	notes = append(notes, rt.secureReachablePreview(ctx, appName, domain, s)...)
	rt.sleepIdlePreview(ctx, previewName, s)
	return notes
}

// applyPreviewDatabaseStrategy connects a single-service preview to its
// database according to the app's strategy. none attaches nothing (the safe
// default), shared attaches the production database, fresh provisions an empty
// instance of the same engine, and seed restores the latest backup of the
// named seed database into one. Multi-service previews and app.yaml databases
// keep their own declarative path (ephemeralInPreviews, isolatedInPreviews).
func (rt *Router) applyPreviewDatabaseStrategy(ctx context.Context, preview *store.PreviewEnvironment, appName, previewName string, gs store.GitSource, s store.PreviewAppSettings) []string {
	if len(gs.Services) > 0 || len(gs.Databases) > 0 {
		return nil
	}
	switch s.DatabaseStrategy {
	case store.PreviewDatabaseStrategyShared, store.PreviewDatabaseStrategyFresh, store.PreviewDatabaseStrategySeed:
	default:
		return nil
	}
	prod, err := rt.apps.GetDesiredService(ctx, appName)
	if err != nil || prod.DatabaseAttachment == nil {
		return []string{"database strategy " + s.DatabaseStrategy + " skipped: the app has no database attached"}
	}
	att := *prod.DatabaseAttachment

	if s.DatabaseStrategy == store.PreviewDatabaseStrategyShared {
		if err := rt.apps.UpdateServiceDatabaseAttachment(ctx, previewName, &att); err != nil {
			rt.logger.Error("api: attach shared preview database failed", slog.String("error", err.Error()), slog.String("preview_app", previewName))
			return []string{"could not attach the shared database"}
		}
		return []string{"connected to the production database " + att.DatabaseName + " (shared strategy)"}
	}

	source := att.DatabaseName
	if s.DatabaseStrategy == store.PreviewDatabaseStrategySeed && s.SeedDatabase != "" {
		source = s.SeedDatabase
	}
	src, err := rt.databases.GetDesiredDatabase(ctx, source)
	if err != nil {
		rt.logger.Error("api: preview database source not found", slog.String("error", err.Error()), slog.String("database", source))
		return []string{"database " + source + " was not found, no preview database was created"}
	}
	db, err := rt.provisionOneEphemeralDatabase(ctx, preview.ID, previewName, previewDatabaseSourceKey, spec.Database{Engine: src.Engine, Version: src.Version})
	if err != nil {
		rt.logger.Error("api: provision preview database failed", slog.String("error", err.Error()), slog.String("preview_app", previewName))
		return []string{"could not create the preview database"}
	}
	rt.attachEphemeralDatabase(ctx, previewName, db)
	if s.DatabaseStrategy == store.PreviewDatabaseStrategyFresh {
		return []string{"empty " + src.Engine + " database " + db.DatabaseName + " created for this preview"}
	}
	return rt.seedPreviewDatabase(ctx, source, db, src.Engine)
}

// seedPreviewDatabase restores the newest succeeded backup of source into the
// preview's database in the background. With no usable backup or restore
// runner the preview keeps its empty database and says so.
func (rt *Router) seedPreviewDatabase(ctx context.Context, source string, db store.PreviewEphemeralDatabase, engine string) []string {
	if rt.cloneRestoreRunner == nil {
		return []string{"seed restore is not configured on this control plane, the preview database is empty"}
	}
	history, err := rt.backupHistory.ListBackupHistory(ctx, source, 25, nil)
	if err != nil {
		rt.logger.Error("api: list seed backups failed", slog.String("error", err.Error()), slog.String("database", source))
		return []string{"could not read backups of " + source + ", the preview database is empty"}
	}
	backupID := ""
	for _, h := range history {
		if h.Status == store.BackupStatusSucceeded {
			backupID = h.ID
			break
		}
	}
	if backupID == "" {
		return []string{"no succeeded backup of " + source + " exists, the preview database is empty"}
	}
	historyID, err := randomCloneRestoreID()
	if err != nil {
		return []string{"could not start the seed restore, the preview database is empty"}
	}
	container, controller := databaseContainerName(db.DatabaseName), databaseControllerName(db.DatabaseName)
	go func() { //nolint:gosec // outlives the webhook request: the restore waits for the new database to come up
		if err := rt.cloneRestoreRunner.RunCloneRestore(context.Background(), historyID, source, db.DatabaseName, backupID, engine, container, controller); err != nil {
			rt.logger.Error("api: preview seed restore failed", slog.String("error", err.Error()), slog.String("database", db.DatabaseName), slog.String("seed", source))
		}
	}()
	return []string{"seeding " + db.DatabaseName + " from the latest backup of " + source}
}
