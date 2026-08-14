package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Target is one container a Collector should poll on each tick.
// ResourceID is the metrics store's own identifier for whatever this
// container belongs to (e.g. "service:web"), deliberately not the raw
// container ID: a redeploy gives a service a new container ID (per
// internal/reconcile/application's deterministic-name-per-image
// design) but its metric history should stay queryable under one
// stable identifier across that change.
type Target struct {
	ResourceID  string
	ContainerID string
}

// StatsSource is the narrow Docker surface a Collector needs: nothing
// about container discovery, only "give me a snapshot for this ID."
// *docker.Client satisfies this structurally. Deliberately not part of
// docker.Runtime (the interface every reconcile controller depends on
// and every existing fake implements): adding a method there would
// require updating every one of those fakes for a capability only this
// package needs, so this stays its own minimal, consumer-defined
// interface instead, the same shape internal/reconcile/application's
// ServiceStore already establishes for the same reason.
type StatsSource interface {
	Stats(ctx context.Context, containerID string) (docker.ContainerStats, error)
}

// Collector polls a caller-supplied set of targets on an interval and
// writes what it collects to a Store.
type Collector struct {
	source   StatsSource
	store    *DB
	interval time.Duration
	logger   *slog.Logger
}

// NewCollector builds a Collector. logger defaults to slog.Default() if
// nil.
func NewCollector(source StatsSource, store *DB, interval time.Duration, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{source: source, store: store, interval: interval, logger: logger}
}

// CollectOnce polls every target once and writes whatever succeeded.
// One target's stats call failing (its container just stopped between
// discovery and polling, for instance) doesn't block the others': the
// same "one broken resource must not stop convergence of everything
// else" principle reconcile.Engine.ReconcileAll already applies to
// controllers, applied here to collection targets. Returns a joined
// error of every target that failed, purely for the caller to log or
// count; a partial failure is not treated as this tick failing overall,
// since the successful samples were still written.
func (c *Collector) CollectOnce(ctx context.Context, targets []Target) error {
	now := time.Now()
	samples := make([]Sample, 0, len(targets)*7) // 7 metrics per container, see sampleValues
	var errs []error

	for _, target := range targets {
		stats, err := c.source.Stats(ctx, target.ContainerID)
		if err != nil {
			errs = append(errs, fmt.Errorf("collect %s: %w", target.ResourceID, err))
			continue
		}
		samples = append(samples, sampleValues(target.ResourceID, now, stats)...)
	}

	if err := c.store.WriteSamples(ctx, samples); err != nil {
		errs = append(errs, fmt.Errorf("write samples: %w", err))
	}

	return errors.Join(errs...)
}

// Run calls CollectOnce every interval until ctx is done. targetsFunc is
// invoked fresh on every tick, not captured once at construction, so
// the polled set stays current as services are created, redeployed, or
// removed between ticks: the same level-triggered, re-derive-every-pass
// principle every reconcile.Controller already follows, applied here to
// "what should be collected" instead of "what should be running."
func (c *Collector) Run(ctx context.Context, targetsFunc func(context.Context) ([]Target, error)) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			targets, err := targetsFunc(ctx)
			if err != nil {
				c.logger.Error("telemetry: list collection targets failed", slog.String("error", err.Error()))
				continue // try again next tick rather than stopping the collector over one bad listing
			}
			if err := c.CollectOnce(ctx, targets); err != nil {
				c.logger.Warn("telemetry: collection tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

// sampleValues maps one container's stats snapshot into the metric_samples
// rows it produces. Metric names here are the store's stable, permanent
// vocabulary: TASKS.md 2.3's query API and 2.4's dashboard read these
// exact strings, so a rename here is a breaking change to both.
func sampleValues(resourceID string, at time.Time, stats docker.ContainerStats) []Sample {
	return []Sample{
		{ResourceID: resourceID, Metric: "cpu_percent", Timestamp: at, Value: stats.CPUPercent},
		{ResourceID: resourceID, Metric: "memory_usage_bytes", Timestamp: at, Value: float64(stats.MemoryUsageBytes)},
		{ResourceID: resourceID, Metric: "memory_limit_bytes", Timestamp: at, Value: float64(stats.MemoryLimitBytes)},
		{ResourceID: resourceID, Metric: "network_rx_bytes", Timestamp: at, Value: float64(stats.NetworkRxBytes)},
		{ResourceID: resourceID, Metric: "network_tx_bytes", Timestamp: at, Value: float64(stats.NetworkTxBytes)},
		{ResourceID: resourceID, Metric: "disk_read_bytes", Timestamp: at, Value: float64(stats.DiskReadBytes)},
		{ResourceID: resourceID, Metric: "disk_write_bytes", Timestamp: at, Value: float64(stats.DiskWriteBytes)},
	}
}
