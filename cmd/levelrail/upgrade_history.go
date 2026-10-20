package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/version"
)

// priorDataGrace separates "this database was created in this boot" from one
// that predates upgrade history.
const priorDataGrace = 2 * time.Minute

// recordUpgradeHistory appends a history row when the running version differs
// from the last recorded one. It never blocks or fails the boot.
func recordUpgradeHistory(ctx context.Context, db *store.DB, obs upgradehistory.Observed, logger *slog.Logger) {
	schema, err := store.MaxSchemaVersion()
	if err != nil {
		schema = -1
	}
	rec := &upgradehistory.Recorder{
		Store: db, DataDir: dataDirFromEnv(), Version: version.Version, Logger: logger,
		PriorData: databasePredatesHistory(ctx, db),
	}
	if b, err := loadBrand(); err == nil {
		rec.Notes = upgradehistory.NewGitHubNotesFetcher(b.RepoSlug())
	}
	e, wrote, err := rec.Record(ctx, schema, obs)
	if err != nil {
		logger.Error("upgrade history: record failed", slog.String("error", err.Error()))
		return
	}
	if wrote {
		logger.Info("upgrade history recorded", slog.String("id", e.ID), slog.String("kind", e.Kind),
			slog.String("from", e.FromVersion), slog.String("to", e.ToVersion))
	}
}

func databasePredatesHistory(ctx context.Context, db *store.DB) bool {
	var first string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MIN(applied_at), '') FROM schema_migrations`).Scan(&first); err != nil || first == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, first)
	return err == nil && time.Since(t) > priorDataGrace
}
