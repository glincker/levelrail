package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/docker"
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
	// NodeID is resolveNodeTransport's own "" means local convention
	// (cmd/levelrail/main.go): which node ContainerID actually runs on,
	// so a StatsSource covering more than one node can route each poll
	// correctly without Target carrying a whole Transport.
	NodeID string
}

// StatsSource is the narrow Docker surface a Collector needs: a
// snapshot for one container on one node. Deliberately not part of
// docker.Runtime (would ripple into every existing fake, the same
// reasoning internal/reconcile/application's ServiceStore already
// gives); cmd/levelrail/main.go's multiNodeStatsSource routes local vs.
// remote nodes behind this one interface.
type StatsSource interface {
	Stats(ctx context.Context, nodeID, containerID string) (docker.ContainerStats, error)
}

// Collector polls a caller-supplied set of targets on an interval and
// writes what it collects to a Store.
type Collector struct {
	source   StatsSource
	store    *DB
	interval time.Duration
	logger   *slog.Logger

	// presence and idleInterval slow polling while nobody watches; nil presence keeps the fixed cadence.
	presence     *Presence
	idleInterval time.Duration
	// intervalFn, when set, is read before every wait so a saved setting applies without a restart.
	intervalFn func() time.Duration
	now        func() time.Time

	// prevCPU replaces a one-shot stats response's own PreCPUStats (see
	// docker.ContainerStats.CPUPercent): a container's first sample
	// reads 0 and self-corrects next tick.
	cpuMu   sync.Mutex
	prevCPU map[string]docker.CPUStatsRaw
}

// LocalStatsSource adapts a docker.StatsInspector (this process's own
// Docker client) into a StatsSource that ignores nodeID, for a
// single-node deployment with no remote agents to route to.
type LocalStatsSource struct {
	docker.StatsInspector
}

// Stats implements StatsSource by ignoring nodeID and calling the
// wrapped local inspector directly.
func (s LocalStatsSource) Stats(ctx context.Context, _, containerID string) (docker.ContainerStats, error) {
	return s.StatsInspector.Stats(ctx, containerID)
}

// NewCollector builds a Collector. logger defaults to slog.Default() if
// nil.
func NewCollector(source StatsSource, store *DB, interval time.Duration, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		source:   source,
		store:    store,
		interval: interval,
		logger:   logger,
		prevCPU:  make(map[string]docker.CPUStatsRaw),
		now:      time.Now,
	}
}

// SetPresence makes Run poll at most every idle while p reports no viewer. Call before Run.
func (c *Collector) SetPresence(p *Presence, idle time.Duration) {
	c.presence, c.idleInterval = p, idle
}

// SetIntervalFunc makes Run read its sampling interval from fn before each wait. Call before Run.
func (c *Collector) SetIntervalFunc(fn func() time.Duration) { c.intervalFn = fn }

func (c *Collector) currentInterval() time.Duration {
	if c.intervalFn != nil {
		if d := c.intervalFn(); d > 0 {
			return d
		}
	}
	return c.interval
}

// dueAt reports whether a tick should collect: always with a viewer, else only once idleInterval passed since the last run.
func (c *Collector) dueAt(last time.Time) bool {
	every := c.currentInterval()
	// Idle polling stays a multiple of the live interval so a slower base never idles faster.
	idle := max(c.idleInterval, 4*every)
	if c.presence == nil || idle <= every || c.presence.Active() {
		return true
	}
	return c.now().Sub(last) >= idle
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
	return c.collect(ctx, targets, true)
}

// watchedOnly keeps the targets whose resource was viewed recently.
func (c *Collector) watchedOnly(targets []Target) []Target {
	var out []Target
	for _, t := range targets {
		if c.presence.ResourceActive(t.ResourceID) {
			out = append(out, t)
		}
	}
	return out
}

// collect is CollectOnce; prune is false for a partial set so unwatched apps keep their CPU baseline.
func (c *Collector) collect(ctx context.Context, targets []Target, prune bool) error {
	now := time.Now()
	samples := make([]Sample, 0, len(targets)*7) // 7 metrics per container, see sampleValues
	var errs []error
	// One sample set per resource: gauges are averaged (limit: max) so they stay per-container,
	// counters are summed. A failed replica drops the resource's counters this tick, so totals never dip.
	combined := map[string]*statsAgg{}
	failed := map[string]bool{}
	var order []string

	for _, target := range targets {
		stats, err := c.source.Stats(ctx, target.NodeID, target.ContainerID)
		if err != nil {
			errs = append(errs, fmt.Errorf("collect %s: %w", target.ResourceID, err))
			failed[target.ResourceID] = true
			continue
		}
		stats.CPUPercent = c.cpuPercent(cpuCacheKey(target.NodeID, target.ContainerID), stats.CPURaw)
		acc, ok := combined[target.ResourceID]
		if !ok {
			acc = &statsAgg{}
			combined[target.ResourceID] = acc
			order = append(order, target.ResourceID)
		}
		acc.add(stats)
	}
	for _, id := range order {
		samples = append(samples, combined[id].samples(id, now, failed[id])...)
	}

	if err := c.store.WriteSamples(ctx, samples); err != nil {
		errs = append(errs, fmt.Errorf("write samples: %w", err))
	}

	if prune {
		c.pruneCPUCache(targets)
	}
	return errors.Join(errs...)
}

// statsAgg folds one resource's containers into a single sample set.
type statsAgg struct {
	n                        int
	cpu                      float64
	mem                      float64
	memLimit                 float64
	rx, tx, diskRead, diskWr float64
}

func (a *statsAgg) add(s docker.ContainerStats) {
	a.n++
	a.cpu += s.CPUPercent
	a.mem += float64(s.MemoryUsageBytes)
	a.memLimit = max(a.memLimit, float64(s.MemoryLimitBytes))
	a.rx += float64(s.NetworkRxBytes)
	a.tx += float64(s.NetworkTxBytes)
	a.diskRead += float64(s.DiskReadBytes)
	a.diskWr += float64(s.DiskWriteBytes)
}

// samples averages the gauges; counters are skipped when a replica failed so the total cannot dip and rebound.
func (a *statsAgg) samples(id string, at time.Time, skipCounters bool) []Sample {
	n := float64(a.n)
	out := []Sample{
		{ResourceID: id, Metric: "cpu_percent", Timestamp: at, Value: a.cpu / n},
		{ResourceID: id, Metric: "memory_usage_bytes", Timestamp: at, Value: a.mem / n},
		{ResourceID: id, Metric: "memory_limit_bytes", Timestamp: at, Value: a.memLimit},
	}
	if skipCounters {
		return out
	}
	return append(out,
		Sample{ResourceID: id, Metric: "network_rx_bytes", Timestamp: at, Value: a.rx},
		Sample{ResourceID: id, Metric: "network_tx_bytes", Timestamp: at, Value: a.tx},
		Sample{ResourceID: id, Metric: "disk_read_bytes", Timestamp: at, Value: a.diskRead},
		Sample{ResourceID: id, Metric: "disk_write_bytes", Timestamp: at, Value: a.diskWr},
	)
}

func (c *Collector) cpuPercent(cacheKey string, raw docker.CPUStatsRaw) float64 {
	c.cpuMu.Lock()
	defer c.cpuMu.Unlock()
	prev, ok := c.prevCPU[cacheKey]
	c.prevCPU[cacheKey] = raw
	if !ok {
		return 0
	}
	return docker.CPUPercent(raw, prev)
}

// cpuCacheKey scopes the previous-sample cache by node as well as
// container ID: two different nodes' Docker daemons can in principle
// hand out the same container ID, and NodeID is free to include in the
// key since it's already carried on every Target.
func cpuCacheKey(nodeID, containerID string) string {
	return nodeID + "\x00" + containerID
}

// pruneCPUCache keeps a long-running control plane's cache from growing
// across every redeploy and removal it has ever seen.
func (c *Collector) pruneCPUCache(targets []Target) {
	live := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		live[cpuCacheKey(t.NodeID, t.ContainerID)] = struct{}{}
	}
	c.cpuMu.Lock()
	defer c.cpuMu.Unlock()
	for key := range c.prevCPU {
		if _, ok := live[key]; !ok {
			delete(c.prevCPU, key)
		}
	}
}

// Run calls CollectOnce every interval until ctx is done. targetsFunc is
// invoked fresh on every tick, not captured once at construction, so
// the polled set stays current as services are created, redeployed, or
// removed between ticks: the same level-triggered, re-derive-every-pass
// principle every reconcile.Controller already follows, applied here to
// "what should be collected" instead of "what should be running."
func (c *Collector) Run(ctx context.Context, targetsFunc func(context.Context) ([]Target, error)) error {
	timer := time.NewTimer(c.currentInterval())
	defer timer.Stop()
	var last time.Time

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			timer.Reset(c.currentInterval())
			due := c.dueAt(last)
			if due {
				last = c.now()
			}
			targets, err := targetsFunc(ctx)
			if err != nil {
				c.logger.Error("telemetry: list collection targets failed", slog.String("error", err.Error()))
				continue // try again next tick rather than stopping the collector over one bad listing
			}
			prune := due
			if !due {
				// Idle fleet: still sample the apps someone has open at the live cadence.
				targets = c.watchedOnly(targets)
				if len(targets) == 0 {
					continue
				}
			}
			if err := c.collect(ctx, targets, prune); err != nil {
				c.logger.Warn("telemetry: collection tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

// sampleValues maps one container's stats snapshot into the metric_samples
// rows it produces. Metric names here are the store's stable, permanent
// vocabulary: the query API and the dashboard read these
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
