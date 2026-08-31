package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultNodeCPUThresholdPercent is how much summed CPU a node's placed
// containers can use before a kind=node_resource_usage rule fires on
// its CPU signal, when no override is configured. cpu_percent
// (collector.go's sampleValues) is already expressed on the same 0-100
// scale nodeSummableMetrics (internal/api/node_metrics.go) sums across
// containers for the dashboard, so this threshold compares directly
// against that same already-user-facing number.
const DefaultNodeCPUThresholdPercent = 80.0

// DefaultNodeMemoryThresholdBytes is how much summed memory a node's
// placed containers can use before a kind=node_resource_usage rule
// fires on its memory signal, when no override is configured. Unlike
// disk (HostDiskCollector) there is no real host memory total anywhere
// in this codebase to divide by: memory_limit_bytes is deliberately
// excluded from nodeSummableMetrics because an unconstrained
// container's reported limit approximates host total memory, not a
// real per-container cap, so summing it would overcount rather than
// give an honest denominator (see nodeSummableMetrics' own doc
// comment). An absolute-bytes floor is the only honest signal available
// today; 4 GiB is a generic starting point operators are expected to
// tune to their own node capacity via APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES.
const DefaultNodeMemoryThresholdBytes = 4 * 1024 * 1024 * 1024

// nodeResourceUsageLookback mirrors diskSpaceLookback: wide enough to
// survive one missed telemetry.Collector tick (15s in
// cmd/levelrail/main.go) without losing the last real reading.
const nodeResourceUsageLookback = 2 * time.Minute

const (
	nodeResourceUsageCPUMetric    = "cpu_percent"
	nodeResourceUsageMemoryMetric = "memory_usage_bytes"
)

// NodeServiceSource is the narrow node-placement surface
// EvaluateNodeResourceUsage needs alongside NodeSource: which services
// are placed on a node, the same lookup internal/api's
// handleQueryNodeMetrics already uses (queryPlacedServiceSamples) to sum
// per-container metrics into a node-level total. *store.DB satisfies
// this structurally.
type NodeServiceSource interface {
	ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]store.DesiredService, error)
}

// EvaluateNodeResourceUsage runs one KindNodeResourceUsage rule against
// every node in nodes and returns its updated evaluation state plus,
// when firing, one human-readable notice line per node whose summed
// CPU and/or memory usage meets or exceeds its threshold, for Engine to
// attach to the outgoing Event.
//
// There is no real host-level CPU/memory collector in this codebase
// (unlike disk, telemetry has no /proc-based host reading): what exists
// is cpu_percent/memory_usage_bytes sampled per container
// (telemetry.Collector) and already summed across a node's placed
// services by internal/api's own node-metrics endpoint
// (nodeSummableMetrics). This evaluator reuses that exact same signal
// rather than inventing a new collector, taking each placed service's
// latest sample (not a time-series sum) since alerting only needs "right
// now," not a chart.
//
// CPU and memory are evaluated together as one rule, not two, because
// they are the same "is this node under load" question from an
// operator's perspective, but they use genuinely different-shaped
// thresholds: CPU is compared as a percent (cpuThresholdPercent, the
// metric's own natural unit), memory as an absolute byte count
// (memoryThresholdBytes), since no honest node-capacity percentage
// exists for memory (see DefaultNodeMemoryThresholdBytes). LastValue
// only ever holds the highest summed CPU percent seen across nodes,
// matching EvaluateNodeDiskSpace's own "one representative number"
// convention; memory readings, having no comparable unit to take a
// single max across with CPU, are surfaced only in the per-node notice
// text.
//
// Firing follows EvaluateNodeDiskSpace's shape: the instant either
// signal is over its threshold on any node, with no ForDuration
// debounce. A node with nothing placed on it, or whose placed services
// have no recent sample for either metric, is skipped, neither
// confirming nor denying the condition. A service whose own metrics
// query fails is logged and skipped, the same "one broken resource must
// not block the rest" stance EvaluateNodeDiskSpace already takes on a
// node.
func EvaluateNodeResourceUsage(ctx context.Context, nodes NodeSource, services NodeServiceSource, metrics MetricsSource, r Rule, cpuThresholdPercent, memoryThresholdBytes float64, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cpuThresholdPercent <= 0 {
		cpuThresholdPercent = DefaultNodeCPUThresholdPercent
	}
	if memoryThresholdBytes <= 0 {
		memoryThresholdBytes = DefaultNodeMemoryThresholdBytes
	}

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
		placed, err := services.ListDesiredServicesByNode(ctx, node.ID)
		if err != nil {
			logger.Warn("alerting: evaluate node_resource_usage rule: list placed services failed, skipping node",
				slog.String("rule_id", r.ID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
			continue
		}
		if len(placed) == 0 {
			continue
		}

		cpuSum, haveCPU := sumLatestServiceMetric(ctx, metrics, placed, nodeResourceUsageCPUMetric, from, now, r.ID, node.ID, logger)
		memSum, haveMem := sumLatestServiceMetric(ctx, metrics, placed, nodeResourceUsageMemoryMetric, from, now, r.ID, node.ID, logger)
		if !haveCPU && !haveMem {
			continue
		}

		if haveCPU && (!haveMaxCPU || cpuSum > maxCPU) {
			maxCPU, haveMaxCPU = cpuSum, true
		}

		overCPU := haveCPU && cpuSum >= cpuThresholdPercent
		overMem := haveMem && memSum >= memoryThresholdBytes
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

// sumLatestServiceMetric sums, across every service in placed, each
// service's own latest sample for metric in [from, now]: not a
// time-series sum like SumAcrossResources (that produces a chart, this
// needs one "right now" total). A service with no recent sample simply
// contributes nothing; haveAny reports whether at least one service did,
// distinguishing "every service reported zero" from "nothing to sum."
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
