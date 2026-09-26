package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// configureTelemetryTiers applies the APP_METRICS_* tier and retention
// settings; call before the store is shared with other goroutines.
func configureTelemetryTiers(db *telemetry.DB) {
	db.Configure(telemetry.TierConfigFromEnv(os.Getenv))
}

// requestSummaryWindow reads APP_REQUESTS_SUMMARY_WINDOW; zero keeps the API default.
func requestSummaryWindow() time.Duration {
	d, err := time.ParseDuration(os.Getenv("APP_REQUESTS_SUMMARY_WINDOW"))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// startTelemetryMaintenance runs the ingress request sampler, the rollup job
// and the retention sweep until ctx is cancelled.
func startTelemetryMaintenance(ctx context.Context, db *telemetry.DB, interval time.Duration, logger *slog.Logger) {
	sampler := telemetry.NewRequestSampler(ingress.DefaultRequestStats(), db, interval, logger)
	go sampler.Run(ctx)
	go telemetry.NewMaintainer(db, 0, 0, logger).Run(ctx)
}
