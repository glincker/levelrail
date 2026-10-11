package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradesched"
)

// upgradeSchedulingOption enables scheduled and automatic upgrades. The
// dispatcher delivers the alerts through the channels an operator connected.
func upgradeSchedulingOption(db *store.DB, dispatcher *alerting.DeployDispatcher) api.Option {
	return api.WithUpgradeScheduling(db, dispatcher, upgradesched.ConfigFromEnv(os.LookupEnv))
}

// startUpgradeScheduler runs the level-triggered upgrade scheduler. It
// reads persisted state on every tick, so a restart resumes where it was.
func startUpgradeScheduler(ctx context.Context, logger *slog.Logger, rt *api.Router) {
	sched := rt.UpgradeScheduler()
	if sched == nil {
		return
	}
	interval := upgradesched.DefaultInterval
	if raw := os.Getenv("APP_UPGRADE_SCHEDULER_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			interval = d
		} else {
			logger.Warn("invalid APP_UPGRADE_SCHEDULER_INTERVAL, using the default", slog.String("value", raw))
		}
	}
	go func() {
		if err := sched.Run(ctx, interval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("upgrade scheduler stopped", slog.String("error", err.Error()))
		}
	}()
}
