package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
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
	notes, _, _ := rt.applyPreviewDatabaseStrategyReady(ctx, preview, appName, previewName, gs, s, false)
	return notes
}

// applyPreviewDatabaseStrategyReady is applyPreviewDatabaseStrategy with an
// optional readiness mode for the migrate step: the seed restore runs to
// completion before returning, and any step that would leave the database
// missing, unattached or empty becomes an error instead of a note. The bool
// is true when this call created the preview database, so only that one may
// be torn down on failure.
func (rt *Router) applyPreviewDatabaseStrategyReady(ctx context.Context, preview *store.PreviewEnvironment, appName, previewName string, gs store.GitSource, s store.PreviewAppSettings, ready bool) ([]string, bool, error) {
	if len(gs.Services) > 0 || len(gs.Databases) > 0 {
		return nil, false, nil
	}
	switch s.DatabaseStrategy {
	case store.PreviewDatabaseStrategyShared, store.PreviewDatabaseStrategyFresh, store.PreviewDatabaseStrategySeed:
	default:
		return nil, false, nil
	}
	prod, err := rt.apps.GetDesiredService(ctx, appName)
	if err != nil || prod.DatabaseAttachment == nil {
		return strategySkippedNew(ready, "database strategy "+s.DatabaseStrategy+" skipped: the app has no database attached")
	}
	att := *prod.DatabaseAttachment

	if s.DatabaseStrategy == store.PreviewDatabaseStrategyShared {
		if err := rt.apps.UpdateServiceDatabaseAttachment(ctx, previewName, &att); err != nil {
			rt.logger.Error("api: attach shared preview database failed", slog.String("error", err.Error()), slog.String("preview_app", previewName))
			return strategySkippedNew(ready, "could not attach the shared database")
		}
		return []string{"connected to the production database " + att.DatabaseName + " (shared strategy)"}, false, nil
	}

	source := att.DatabaseName
	if s.DatabaseStrategy == store.PreviewDatabaseStrategySeed && s.SeedDatabase != "" {
		source = s.SeedDatabase
	}
	src, err := rt.databases.GetDesiredDatabase(ctx, source)
	if err != nil {
		rt.logger.Error("api: preview database source not found", slog.String("error", err.Error()), slog.String("database", source))
		return strategySkippedNew(ready, "database "+source+" was not found, no preview database was created")
	}
	db, created, err := rt.provisionOneEphemeralDatabase(ctx, preview.ID, previewName, previewDatabaseSourceKey, spec.Database{Engine: src.Engine, Version: src.Version})
	if err != nil {
		rt.logger.Error("api: provision preview database failed", slog.String("error", err.Error()), slog.String("preview_app", previewName))
		return strategySkippedNew(ready, "could not create the preview database")
	}
	attached := rt.attachEphemeralDatabase(ctx, previewName, db)
	if ready && !attached {
		return nil, created, errors.New("migration skipped: the preview database could not be attached to the preview")
	}
	if s.DatabaseStrategy == store.PreviewDatabaseStrategyFresh {
		return []string{"empty " + src.Engine + " database " + db.DatabaseName + " created for this preview"}, created, nil
	}
	notes, seedErr := rt.seedPreviewDatabase(ctx, source, db, src.Engine, ready)
	return notes, created, seedErr
}

// strategySkippedNew is strategySkipped for returns that created no database.
func strategySkippedNew(ready bool, msg string) ([]string, bool, error) {
	notes, err := strategySkipped(ready, msg)
	return notes, false, err
}

// strategySkipped is a note normally, but an error when a migration needs
// the database the strategy failed to provide.
func strategySkipped(ready bool, msg string) ([]string, error) {
	if ready {
		return nil, errors.New("migration skipped: " + msg)
	}
	return []string{msg}, nil
}

const (
	envPreviewSeedTimeout     = "APP_PREVIEW_SEED_TIMEOUT"
	defaultPreviewSeedTimeout = 30 * time.Minute
)

// markSeedFailed records on the preview database row why seeding stopped, so
// the preview stops reading as "seeding" forever.
func (rt *Router) markSeedFailed(id string, cause error) {
	if rt.previewEnvironments == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.previewEnvironments.UpdatePreviewEphemeralDatabaseStatus(ctx, id, store.PreviewEphemeralDatabaseStatusSeedFailed, cause.Error(), now); err != nil {
		rt.logger.Error("api: record preview seed failure failed", slog.String("error", err.Error()), slog.String("database_id", id))
	}
}

// seedPreviewDatabase restores the newest succeeded backup of source into the
// preview's database in the background. With no usable backup or restore
// runner the preview keeps its empty database and says so.
func (rt *Router) seedPreviewDatabase(ctx context.Context, source string, db store.PreviewEphemeralDatabase, engine string, wait bool) ([]string, error) {
	if rt.cloneRestoreRunner == nil {
		return seedEmpty(wait, "seed restore is not configured on this control plane, the preview database is empty")
	}
	if err := backup.CheckWorkDirSpace(); err != nil {
		return seedEmpty(wait, "not enough free disk to seed "+db.DatabaseName+" from "+source+", the preview database is empty")
	}
	history, err := rt.backupHistory.ListBackupHistory(ctx, source, 25, nil)
	if err != nil {
		rt.logger.Error("api: list seed backups failed", slog.String("error", err.Error()), slog.String("database", source))
		return seedEmpty(wait, "could not read backups of "+source+", the preview database is empty")
	}
	backupID := ""
	for _, h := range history {
		if h.Status == store.BackupStatusSucceeded {
			backupID = h.ID
			break
		}
	}
	if backupID == "" {
		return seedEmpty(wait, "no succeeded backup of "+source+" exists, the preview database is empty")
	}
	historyID, err := randomCloneRestoreID()
	if err != nil {
		return seedEmpty(wait, "could not start the seed restore, the preview database is empty")
	}
	container, controller := databaseContainerName(db.DatabaseName), databaseControllerName(db.DatabaseName)
	seedTimeout := envDuration(envPreviewSeedTimeout, defaultPreviewSeedTimeout)
	note := "seeding " + db.DatabaseName + " from the latest backup of " + source
	if wait {
		// The migration must see the restored data, so the restore finishes first.
		seedCtx, cancel := context.WithTimeout(ctx, seedTimeout)
		defer cancel()
		if err := rt.cloneRestoreRunner.RunCloneRestore(seedCtx, historyID, source, db.DatabaseName, backupID, engine, container, controller); err != nil {
			rt.logger.Error("api: preview seed restore failed", slog.String("error", err.Error()), slog.String("database", db.DatabaseName), slog.String("seed", source))
			rt.markSeedFailed(db.ID, err)
			return nil, errors.New("migration skipped: the seed restore of " + db.DatabaseName + " failed")
		}
		return []string{"seeded " + db.DatabaseName + " from the latest backup of " + source}, nil
	}
	go func() { //nolint:gosec // outlives the webhook request: the restore waits for the new database to come up
		seedCtx, cancel := context.WithTimeout(context.Background(), seedTimeout)
		defer cancel()
		if err := rt.cloneRestoreRunner.RunCloneRestore(seedCtx, historyID, source, db.DatabaseName, backupID, engine, container, controller); err != nil {
			rt.logger.Error("api: preview seed restore failed", slog.String("error", err.Error()), slog.String("database", db.DatabaseName), slog.String("seed", source))
			rt.markSeedFailed(db.ID, err)
		}
	}()
	return []string{note}, nil
}

// seedEmpty is the note for a seed that could not start (the preview keeps an
// empty database), or an error when a migration must not run on that.
func seedEmpty(wait bool, msg string) ([]string, error) {
	if wait {
		return nil, errors.New("migration skipped: " + msg)
	}
	return []string{msg}, nil
}
