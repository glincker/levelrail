package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

const reapBootTimeout = 30 * time.Second

// reapAbandonedUpgrades turns an upgrade attempt whose worker died into a
// failed one at boot, so the dashboard never shows it running forever.
func reapAbandonedUpgrades(dataDir string, db selfupgrade.AttemptStore, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), reapBootTimeout)
	defer cancel()
	closed, err := selfupgrade.ReapAndImport(ctx, dataDir, db, time.Now(), selfupgrade.LimitsFromEnv(os.Getenv))
	if err != nil {
		logger.Warn("abandoned upgrade cleanup failed", slog.String("error", err.Error()))
		return
	}
	for _, a := range closed {
		logger.Warn("abandoned upgrade attempt closed at boot", slog.String("attempt", a.ID), slog.String("step", a.FailedStep))
	}
}
