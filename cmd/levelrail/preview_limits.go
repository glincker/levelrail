package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
)

const (
	defaultPreviewMaxPerApp = 10
	defaultPreviewMaxTotal  = 50
)

// previewLimits reads APP_PREVIEW_ENV_MAX_PER_APP and
// APP_PREVIEW_ENV_MAX_TOTAL. 0 disables a cap; unset or invalid uses the default.
func previewLimits(logger *slog.Logger) api.PreviewLimits {
	return api.PreviewLimits{
		MaxPerApp: nonNegativeEnvInt(logger, "APP_PREVIEW_ENV_MAX_PER_APP", defaultPreviewMaxPerApp),
		MaxTotal:  nonNegativeEnvInt(logger, "APP_PREVIEW_ENV_MAX_TOTAL", defaultPreviewMaxTotal),
	}
}

func nonNegativeEnvInt(logger *slog.Logger, name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		logger.Warn("invalid "+name+", using the default", slog.String("value", raw), slog.Int("default", def))
		return def
	}
	return v
}

// previewStuckAfter reads APP_PREVIEW_STUCK_AFTER: how long a preview may sit
// in deploying before the startup sweep marks it failed. 0 keeps api's default.
func previewStuckAfter(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_PREVIEW_STUCK_AFTER")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid APP_PREVIEW_STUCK_AFTER, using the default", slog.String("value", raw))
		return 0
	}
	return d
}

// sweepOrphanPreviewsOnce repairs preview state a crash left behind.
func sweepOrphanPreviewsOnce(ctx context.Context, logger *slog.Logger, r *api.Router) {
	res, err := r.SweepOrphanPreviews(ctx)
	if err != nil {
		logger.Warn("orphan preview sweep had errors", slog.String("error", err.Error()))
	}
	if res.StuckDeploys > 0 || res.OrphanedServices > 0 {
		logger.Info("orphan preview sweep repaired state", slog.Int("stuck_deploys", res.StuckDeploys), slog.Int("orphaned_services", res.OrphanedServices))
	}
}
