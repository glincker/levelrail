package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/takeover"
)

// applyTakeoverBoot makes a stored same-server takeover win over the
// drop-in's listen addresses before anything reads them, and turns a run that
// never verified into a rollback so the old addresses return.
func applyTakeoverBoot(ctx context.Context, db *store.DB, logger *slog.Logger) {
	d, err := takeover.PrepareBoot(ctx, takeover.DBStore{DB: db}, time.Now().UTC())
	if err != nil {
		logger.Warn("port takeover state not read", slog.String("error", err.Error()))
		return
	}
	switch d.Action {
	case takeover.BootApply:
		_ = os.Setenv("APP_INGRESS_HTTP_ADDR", d.Run.TargetHTTP)
		_ = os.Setenv("APP_INGRESS_HTTPS_ADDR", d.Run.TargetHTTPS)
		// systemd's sockets sit on the old ports; ignore them so the new addresses bind directly.
		_ = os.Setenv(ingressdriver.EnvSocketActivation, "false")
		logger.Info("port takeover: listening on the takeover addresses",
			slog.String("run_id", d.Run.ID), slog.String("http", d.Run.TargetHTTP), slog.String("https", d.Run.TargetHTTPS))
	case takeover.BootRollback:
		logger.Warn("port takeover: rolling back to the previous addresses",
			slog.String("run_id", d.Run.ID), slog.String("reason", d.Reason))
	}
}
