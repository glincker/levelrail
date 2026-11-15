package main

import (
	"context"
	"log/slog"
	"os/exec"

	"github.com/GLINCKER/levelrail/internal/hostprotect"
)

// applyHostProtection writes the OOM drop-in before the upgrade's own restart, so
// that restart is the one that applies it. It never fails the upgrade.
func applyHostProtection(ctx context.Context, name string, logger *slog.Logger) {
	reload := func(ctx context.Context) error {
		return exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	}
	res, err := hostprotect.DefaultHost(name, reload).Apply(ctx)
	if err != nil {
		logger.Warn("host protection drop-in not applied", slog.String("error", err.Error()))
		return
	}
	if res == hostprotect.Wrote {
		logger.Info("host protection drop-in written, applies on the upgrade restart")
	}
}
