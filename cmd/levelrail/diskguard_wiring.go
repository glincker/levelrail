package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/diskguard"
	"github.com/glincker/levelrail/internal/keyreminder"
)

// startDiskGuard watches the data dir and Docker root, refuses risky operations below the critical floor and notifies on a drop.
func startDiskGuard(ctx context.Context, logger *slog.Logger, dataDir string, channels *alerting.DB, notifier keyreminder.Notifier, router *api.Router, db api.NodeGuardrailStore) {
	g := diskguard.New(dataDir, diskguard.DockerRootFromEnv(os.LookupEnv), diskguard.ThresholdsFromEnv(os.LookupEnv), nil, logger)
	g.SetThresholdSource(diskThresholdSource(router))
	bindBuildMemorySetting(router)
	g.Notify = func(ctx context.Context, text string) {
		list, err := channels.ListNotificationChannels(ctx)
		if err != nil {
			logger.Warn("diskguard: list channels failed", slog.String("error", err.Error()))
			return
		}
		for _, c := range list {
			if !c.Enabled || c.ProjectID != "" {
				continue
			}
			if err := notifier.SendNotice(ctx, c, text); err != nil {
				if errors.Is(err, alerting.ErrNoticeSkipped) {
					continue
				}
				logger.Warn("diskguard: notice failed", slog.String("channel_id", c.ID), slog.String("error", err.Error()))
			}
		}
	}
	router.SetDiskGuard(g)
	router.SetNodeGuardrails(db, g.Notify)
	go router.RunNodeGuardrailWatcher(ctx)
	go func() {
		if err := g.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("disk guard stopped", slog.String("error", err.Error()))
		}
	}()
}
