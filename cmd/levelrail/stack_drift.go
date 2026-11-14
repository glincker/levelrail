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
// revision still matches live state. It never applies anything; the live
// interval setting (0 = off) is re-read every tick.
func startStackDrift(ctx context.Context, logger *slog.Logger, db *store.DB, rt *api.Router) {
	c := &stackdrift.Checker{
		Store:       rt.StackDriftStore(db),
		Live:        rt.StackDriftLive,
		ProjectName: rt.StackProjectName,
		Logger:      logger,
		StartDelay:  time.Minute,
		Interval:    rt.StackDriftInterval,
		SummaryMax:  rt.StackDriftSummaryMax,
	}
	rt.RevokeStaleStackDriftTokens(ctx)
	go c.RunLive(ctx)
}
