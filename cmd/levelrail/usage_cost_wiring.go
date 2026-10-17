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

// spendCapInterval is how often per-app spend caps are re-evaluated.
const spendCapInterval = 2 * time.Minute

type spendCapTicker interface {
	SpendCapTick(ctx context.Context)
}

// runSpendCaps re-derives every app's cap enforcement until ctx ends.
func runSpendCaps(ctx context.Context, r spendCapTicker) {
	r.SpendCapTick(ctx)
	t := time.NewTicker(spendCapInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.SpendCapTick(ctx)
		}
	}
}
