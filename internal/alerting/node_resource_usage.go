package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultNodeCPUThresholdPercent is the summed node CPU percent that fires a node_resource_usage rule when unset.
const DefaultNodeCPUThresholdPercent = 80.0

// DefaultNodeMemoryThresholdBytes is the summed node memory that fires a node_resource_usage rule when unset.
// No honest host memory total exists to compare against, so this absolute floor should be tuned per node.
const DefaultNodeMemoryThresholdBytes = 4 * 1024 * 1024 * 1024

// nodeResourceUsageLookback mirrors diskSpaceLookback: wide enough to
// survive one missed telemetry.Collector tick (15s in
// cmd/levelrail/main.go) without losing the last real reading.
const nodeResourceUsageLookback = 2 * time.Minute

const (
	nodeResourceUsageCPUMetric    = "cpu_percent"
	nodeResourceUsageMemoryMetric = "memory_usage_bytes"
)

// NodeServiceSource lists the services placed on a node; *store.DB satisfies it structurally.
type NodeServiceSource interface {
	ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]store.DesiredService, error)
}

// EvaluateNodeResourceUsage checks each node's summed CPU and memory against its threshold, returning one notice per breaching node.
// Memory is compared in absolute bytes, since no host total exists; LastValue holds only the max CPU percent.
func EvaluateNodeResourceUsage(ctx context.Context, nodes NodeSource, services NodeServiceSource, metrics MetricsSource, r Rule, cpuThresholdPercent, memoryThresholdBytes float64, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cpuThresholdPercent, memoryThresholdBytes = resolveNodeResourceThresholds(cpuThresholdPercent, memoryThresholdBytes)

	all, err := nodes.ListNodes(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list nodes: %w", r.ID, err)
	}

	next := r
	next.LastEvaluatedAt = &now

	if len(all) == 0 {
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next, nil, nil
	}

	from := now.Add(-nodeResourceUsageLookback)

	var (
		notices      []string
		anyUnhealthy bool
		maxCPU       float64
		haveMaxCPU   bool
	)

	for _, node := range all {
		cpuSum, memSum, haveCPU, haveMem, overCPU, overMem := evaluateNodeResourceUsageForNode(
			ctx, services, metrics, r.ID, node, cpuThresholdPercent, memoryThresholdBytes, from, now, logger)
		if !haveCPU && !haveMem {
			continue
		}

		if haveCPU && (!haveMaxCPU || cpuSum > maxCPU) {
			maxCPU, haveMaxCPU = cpuSum, true
		}

		if overCPU || overMem {
			anyUnhealthy = true
			notices = append(notices, nodeResourceUsageNotice(node, overCPU, cpuSum, cpuThresholdPercent, overMem, memSum, memoryThresholdBytes))
		}
	}

	if haveMaxCPU {
		v := maxCPU
		next.LastValue = &v
	}

	return advanceState(next, r, anyUnhealthy, 0, now), notices, nil
}

// resolveNodeResourceThresholds substitutes defaults for zero-valued thresholds.
func resolveNodeResourceThresholds(cpuThresholdPercent, memoryThresholdBytes float64) (float64, float64) {
	if cpuThresholdPercent <= 0 {
		cpuThresholdPercent = DefaultNodeCPUThresholdPercent
	}
	if memoryThresholdBytes <= 0 {
		memoryThresholdBytes = DefaultNodeMemoryThresholdBytes
	}
	return cpuThresholdPercent, memoryThresholdBytes
}

// sumLatestServiceMetric sums each placed service's latest sample, not a time series, since alerting needs the current total.
func sumLatestServiceMetric(ctx context.Context, metrics MetricsSource, placed []store.DesiredService, metric string, from, now time.Time, ruleID, nodeID string, logger *slog.Logger) (sum float64, haveAny bool) {
	for _, svc := range placed {
		samples, err := metrics.QueryMetrics(ctx, "service:"+svc.Name, metric, from, now)
		if err != nil {
			logger.Warn("alerting: evaluate node_resource_usage rule: query service metric failed, skipping service",
				slog.String("rule_id", ruleID), slog.String("node_id", nodeID), slog.String("service", svc.Name),
				slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		if len(samples) == 0 {
			continue
		}
		sum += samples[len(samples)-1].Value
		haveAny = true
	}
	return sum, haveAny
}

// evaluateNodeResourceUsageForNode is one node's check, shared by the loop and the live single-node check.
func evaluateNodeResourceUsageForNode(ctx context.Context, services NodeServiceSource, metrics MetricsSource, ruleID string, node store.Node, cpuThresholdPercent, memoryThresholdBytes float64, from, now time.Time, logger *slog.Logger) (cpuSum, memSum float64, haveCPU, haveMem, overCPU, overMem bool) {
	placed, err := services.ListDesiredServicesByNode(ctx, node.ID)
	if err != nil {
		logger.Warn("alerting: evaluate node_resource_usage rule: list placed services failed, skipping node",
			slog.String("rule_id", ruleID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
		return 0, 0, false, false, false, false
	}
	if len(placed) == 0 {
		return 0, 0, false, false, false, false
	}

	cpuSum, haveCPU = sumLatestServiceMetric(ctx, metrics, placed, nodeResourceUsageCPUMetric, from, now, ruleID, node.ID, logger)
	memSum, haveMem = sumLatestServiceMetric(ctx, metrics, placed, nodeResourceUsageMemoryMetric, from, now, ruleID, node.ID, logger)
	if !haveCPU && !haveMem {
		return cpuSum, memSum, false, false, false, false
	}

	overCPU = haveCPU && cpuSum >= cpuThresholdPercent
	overMem = haveMem && memSum >= memoryThresholdBytes
	return cpuSum, memSum, haveCPU, haveMem, overCPU, overMem
}

func nodeResourceUsageNotice(node store.Node, overCPU bool, cpuPercent, cpuThreshold float64, overMem bool, memBytes, memThreshold float64) string {
	name := node.Name
	if name == "" {
		name = node.ID
	}

	var parts []string
	if overCPU {
		parts = append(parts, fmt.Sprintf("CPU %.1f%% used (threshold %.1f%%)", cpuPercent, cpuThreshold))
	}
	if overMem {
		const gib = 1 << 30
		parts = append(parts, fmt.Sprintf("memory %.2f GiB used (threshold %.2f GiB)", memBytes/gib, memThreshold/gib))
	}
	return fmt.Sprintf("%s: %s", name, strings.Join(parts, ", "))
}
