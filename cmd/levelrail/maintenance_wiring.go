package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startScheduledMaintenance enables the scheduled maintenance routes and
// starts the lead-time announcer and the timed domain maintenance expiry.
func startScheduledMaintenance(ctx context.Context, logger *slog.Logger, db *store.DB, channels *alerting.DB, notifier maintenance.Notifier, router *api.Router) {
	router.SetScheduledMaintenance(db)
	announcer := &maintenance.Announcer{Store: db, Channels: channels, Notifier: notifier, Logger: logger}
	go func() {
		if err := announcer.Run(ctx, maintenance.DefaultInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("maintenance announcer stopped", slog.String("error", err.Error()))
		}
	}()
	expirer := &maintenance.DomainExpirer{Store: db, Silencer: &maintenance.Silencer{Store: channels}, Nudge: router.NudgeReconciler, Logger: logger}
	go func() {
		if err := expirer.Run(ctx, maintenance.DefaultInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("domain maintenance expiry stopped", slog.String("error", err.Error()))
		}
	}()
}
