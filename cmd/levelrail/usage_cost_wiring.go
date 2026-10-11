package main

import (
	"context"
	"time"
)

// usageRollupInterval is how often missing daily cost snapshots are
// written; one UTC day is only ever snapshotted once, so this is cheap.
const usageRollupInterval = time.Hour

type usageRoller interface {
	UsageRollupTick(ctx context.Context)
}

// runUsageRollup snapshots cost history until ctx ends, so it outlives
// telemetry retention.
func runUsageRollup(ctx context.Context, r usageRoller) {
	r.UsageRollupTick(ctx)
	t := time.NewTicker(usageRollupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.UsageRollupTick(ctx)
		}
	}
}
