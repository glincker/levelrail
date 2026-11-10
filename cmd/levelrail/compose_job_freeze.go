package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// composeJobStore is what the compose schedule freeze check reads.
type composeJobStore interface {
	deploy.FreezeStore
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// composeJobFreezeReason pauses a compose member's scheduled tick while a freeze
// covers the member, its stack or everything, as app schedules do. Plain app
// jobs keep running. A failed freeze lookup pauses the tick: running during an
// unknown freeze is the worse mistake for a stack.
func composeJobFreezeReason(ctx context.Context, db composeJobStore, logger *slog.Logger, member string, now time.Time) string {
	svc, err := db.GetDesiredService(ctx, member)
	if err != nil || svc == nil || svc.AppID == "" || svc.AppID == svc.Name {
		return ""
	}
	for _, scope := range []string{member, svc.AppID} {
		status, err := deploy.CheckFreeze(ctx, db, scope, now)
		if err != nil {
			logger.Warn("compose schedule: freeze check failed, tick skipped", slog.String("service", member), slog.String("error", err.Error()))
			return "skipped: freeze check failed"
		}
		if status.Frozen {
			return "skipped: deploy freeze active until " + status.Until.UTC().Format(time.RFC3339) + " (" + status.Reason + ")"
		}
	}
	return ""
}
