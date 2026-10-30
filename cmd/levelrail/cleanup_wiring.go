package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/cleanup"
	"github.com/GLINCKER/levelrail/internal/keyreminder"
	"github.com/GLINCKER/levelrail/internal/store"
)

// setupCleanup registers the cleanup targets, enables the /api/v1/cleanup routes and starts the scheduler.
// The scheduler stops with ctx and does nothing inside its startup delay or while cleanup.enabled is off.
func setupCleanup(ctx context.Context, logger *slog.Logger, db *store.DB, alertingDB *alerting.DB, notifier keyreminder.Notifier, router *api.Router) {
	settings := router.RuntimeSettings()
	cleanup.RegisterBuiltins(cleanup.Default, cleanup.BuiltinDeps{Store: db, Alerts: alertingDB, Tokens: db, Janitor: db, Retention: db, Settings: settings})
	center := cleanup.New(cleanup.Config{
		Registry: cleanup.Default,
		Store:    db,
		Settings: settings,
		Audit:    db,
		Emit:     router.EmitEvent,
		Notify:   channelNotice(alertingDB, notifier),
		Limits:   cleanup.LimitsFromEnv(os.LookupEnv),
		Logger:   logger,
	})
	router.SetCleanup(center)
	go center.Start(ctx)
}

// channelNotice sends one plain-text notice to every enabled notification channel and fails only when none accepted it.
func channelNotice(channels keyreminder.Channels, notifier keyreminder.Notifier) func(context.Context, string) error {
	return func(ctx context.Context, text string) error {
		list, err := channels.ListNotificationChannels(ctx)
		if err != nil {
			return fmt.Errorf("list notification channels: %w", err)
		}
		sent := 0
		var lastErr error
		for _, c := range list {
			if !c.Enabled {
				continue
			}
			if err := notifier.SendNotice(ctx, c, text); err != nil {
				lastErr = err
				continue
			}
			sent++
		}
		if sent == 0 && lastErr != nil {
			return fmt.Errorf("send notice: %w", lastErr)
		}
		return nil
	}
}
