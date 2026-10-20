package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// seedDefaultAlerts creates the default alert rules once; APP_DEFAULT_ALERTS=off skips it.
func seedDefaultAlerts(ctx context.Context, logger *slog.Logger, db *alerting.DB) {
	if strings.EqualFold(os.Getenv("APP_DEFAULT_ALERTS"), "off") {
		return
	}
	cfg := alerting.DefaultsConfigFromEnv(os.Getenv)
	if err := db.SeedDefaults(ctx, cfg); err != nil {
		logger.Warn("seeding default alert rules failed, will retry next boot", slog.String("error", err.Error()))
	}
}
