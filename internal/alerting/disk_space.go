package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DefaultNodeDiskSpaceThresholdPercent is the percent-used level a node_disk_space rule fires at when none is configured.
// Percent-used, not a free-byte floor, because fixed floors do not scale across disk sizes.
const DefaultNodeDiskSpaceThresholdPercent = 90.0

// Wider than the 15s collection tick, so one missed tick does not lose the last real reading.
const diskSpaceLookback = 2 * time.Minute

// Duplicated from nodePatchResourceID on purpose: each evaluator owns its resource-ID helper, so this package does not import internal/api.
func nodeDiskResourceID(nodeID string) string {
	return "node:" + nodeID
}

// EvaluateNodeDiskSpace fires as soon as any node's used-disk percent meets thresholdPercent, with no debounce.
// Nodes without a recent sample or with a non-positive total are skipped; a failing node query is logged and skipped.
func EvaluateNodeDiskSpace(ctx context.Context, nodes NodeSource, metrics MetricsSource, r Rule, thresholdPercent float64, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if thresholdPercent <= 0 {
		thresholdPercent = DefaultNodeDiskSpaceThresholdPercent
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

	from := now.Add(-diskSpaceLookback)

	var (
		notices      []string
		anyUnhealthy bool
		maxValue     float64
		haveMaxValue bool
	)

	for _, node := range all {
		percent, have, firing := evaluateNodeDiskSpaceForNode(ctx, metrics, r.ID, node, thresholdPercent, from, now, logger)
		if !have {
			continue
		}

		if !haveMaxValue || percent > maxValue {
			maxValue, haveMaxValue = percent, true
		}

		if firing {
			anyUnhealthy = true
			notices = append(notices, diskSpaceNotice(node, percent, thresholdPercent))
		}
	}

	if haveMaxValue {
		v := maxValue
		next.LastValue = &v
	}

	return advanceState(next, r, anyUnhealthy, 0, now), notices, nil
}

// evaluateNodeDiskSpaceForNode is one node's check, shared with the live single-node check in node_alert_status.go.
// have is false when there is no recent sample.
func evaluateNodeDiskSpaceForNode(ctx context.Context, metrics MetricsSource, ruleID string, node store.Node, thresholdPercent float64, from, now time.Time, logger *slog.Logger) (percent float64, have, firing bool) {
	resourceID := nodeDiskResourceID(node.ID)

	usedSamples, err := metrics.QueryMetrics(ctx, resourceID, telemetry.MetricDiskUsedBytes, from, now)
	if err != nil {
		logger.Warn("alerting: evaluate node_disk_space rule: query node disk usage failed, skipping node",
			slog.String("rule_id", ruleID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
		return 0, false, false
	}
	totalSamples, err := metrics.QueryMetrics(ctx, resourceID, telemetry.MetricDiskTotalBytes, from, now)
	if err != nil {
		logger.Warn("alerting: evaluate node_disk_space rule: query node disk capacity failed, skipping node",
			slog.String("rule_id", ruleID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
		return 0, false, false
	}
	if len(usedSamples) == 0 || len(totalSamples) == 0 {
		return 0, false, false
	}

	total := totalSamples[len(totalSamples)-1].Value
	if total <= 0 {
		return 0, false, false
	}
	used := usedSamples[len(usedSamples)-1].Value
	percent = used / total * 100
	return percent, true, percent >= thresholdPercent
}

func diskSpaceNotice(node store.Node, percent, threshold float64) string {
	name := node.Name
	if name == "" {
		name = node.ID
	}
	return fmt.Sprintf("%s: disk %.1f%% used (threshold %.1f%%)", name, percent, threshold)
}
