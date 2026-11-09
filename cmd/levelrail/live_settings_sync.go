package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spool"
)

// liveSettingsEvery is how soon a saved change reaches the package-level knobs below.
const liveSettingsEvery = 15 * time.Second

// applyLiveSettings pushes saved values into packages that hold atomics instead of a settings handle.
func applyLiveSettings(ctx context.Context, rs *runtimesettings.Service) {
	if rs == nil {
		return
	}
	docker.SetPullConcurrency(rs.Int(ctx, runtimesettings.KeyImagesPullConcurrency))
	spool.SetMaxAge(int64(rs.Int(ctx, runtimesettings.KeySpoolMaxAgeSeconds)))
	spool.SetQuota(int64(rs.Int(ctx, runtimesettings.KeySpoolQuotaBytes)))
	api.SetExpensiveReadTTL(int64(rs.Int(ctx, runtimesettings.KeyAPIExpensiveReadTTL)))
}

// runLiveSettings applies once at boot, then on a short tick so changes need no restart.
func runLiveSettings(ctx context.Context, rs *runtimesettings.Service, logger *slog.Logger) {
	applyLiveSettings(ctx, rs)
	err := footprint.Every(ctx, "live settings apply", liveSettingsEvery, logger, func(c context.Context) error {
		applyLiveSettings(c, rs)
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
		logger.Error("live settings apply stopped", slog.String("error", err.Error()))
	}
}
