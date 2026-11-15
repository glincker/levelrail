package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// recoverInterruptedDeploys settles deploys the previous process left running
// and records one event per affected app. Failures are logged, never fatal.
func recoverInterruptedDeploys(ctx context.Context, db *store.DB, logger *slog.Logger) {
	recovered, err := db.RecoverInterruptedDeployAttempts(ctx, time.Now())
	if err != nil {
		logger.Error("recover interrupted deploy attempts", slog.String("error", err.Error()))
	}
	for _, r := range recovered {
		logger.Warn("recovered deploy attempt left running by a previous process",
			slog.String("attempt_id", r.ID), slog.String("app", r.ServiceName), slog.String("outcome", r.Outcome))
		if r.Outcome == store.RecoveredRequeued {
			continue
		}
		title := "Deploy interrupted by a control plane restart"
		detail := store.DeployReasonInterrupted
		if r.Outcome == store.RecoveredConverging {
			title, detail = "Deploy finished cutting over before a control plane restart", store.DeployReasonConverging
		}
		e := store.AppEvent{AppName: r.ServiceName, Kind: store.AppEventDeployInterrupted, Actor: "system", Title: title, Detail: detail}
		if err := db.AddAppEvent(ctx, e); err != nil {
			logger.Error("record interrupted deploy event", slog.String("error", err.Error()), slog.String("app", r.ServiceName))
		}
	}
}
