package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
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

// routeDefaultAlerts sends seeded rules to the first admin's email when an email provider is configured; otherwise they stay in-app.
func routeDefaultAlerts(ctx context.Context, logger *slog.Logger, adb *alerting.DB, db *store.DB, sender *email.DynamicSender) {
	if strings.EqualFold(os.Getenv("APP_DEFAULT_ALERTS"), "off") || sender == nil || !sender.Configured(ctx) {
		return
	}
	users, err := db.ListUsers(ctx)
	if err != nil {
		logger.Warn("routing default alerts: list users failed", slog.String("error", err.Error()))
		return
	}
	admin := ""
	for _, u := range users {
		if u.IsFirstUser && u.Email != "" {
			admin = u.Email
			break
		}
	}
	if _, err := adb.RouteDefaultRules(ctx, admin); err != nil {
		logger.Warn("routing default alerts to admin email failed", slog.String("error", err.Error()))
	}
}
