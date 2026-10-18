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

type autoscaleTicker interface {
	AutoscaleTick(ctx context.Context)
}

// runAutoscaler re-derives every autoscaled app's replica count until ctx ends.
func runAutoscaler(ctx context.Context, r autoscaleTicker, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.AutoscaleTick(ctx)
		}
	}
}

// costBudgetInterval is how often named cost budgets are re-evaluated.
const costBudgetInterval = 2 * time.Minute

type costBudgetTicker interface {
	CostBudgetTick(ctx context.Context)
}

// runCostBudgets re-derives every named budget's state until ctx ends.
func runCostBudgets(ctx context.Context, r costBudgetTicker) {
	r.CostBudgetTick(ctx)
	t := time.NewTicker(costBudgetInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.CostBudgetTick(ctx)
		}
	}
}
