package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/stackdrift"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startStackDrift records, on an interval, whether each stack's latest applied
// revision still matches live state. It never applies anything;
// APP_STACK_DRIFT_INTERVAL=0 turns it off.
func startStackDrift(ctx context.Context, logger *slog.Logger, db *store.DB, rt *api.Router) {
	interval := stackdrift.IntervalFromEnv()
	c := &stackdrift.Checker{
		Store:       db,
		Live:        rt.StackDriftLive,
		ProjectName: rt.StackProjectName,
		Logger:      logger,
		StartDelay:  time.Minute,
	}
	rt.RevokeStaleStackDriftTokens(ctx)
	go c.Run(ctx, interval)
}
