package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DefaultNodeDiskSpaceThresholdPercent is how full (percent of disk
// used) a node can get before a kind=node_disk_space rule fires, when no
// override is configured. HostDiskCollector only ever reports total and
// used bytes, so percent-used is the natural signal to threshold on
// (unlike doctorCheckDiskSpace's fixed free-byte floor, which does not
// scale across wildly different disk sizes).
const DefaultNodeDiskSpaceThresholdPercent = 90.0

// diskSpaceLookback mirrors patchStatusLookback's reasoning but tighter,
// since HostDiskCollector samples on metricsCollectionInterval (15s in
// cmd/levelrail/main.go) rather than hourly: wide enough to survive one
// missed tick without losing the last real reading.
const diskSpaceLookback = 2 * time.Minute

// nodeDiskResourceID duplicates nodePatchResourceID's own logic rather
// than calling it: each evaluator in this package owns its resource-ID
// helper independently, the same convention nodePatchResourceID's own
// doc comment establishes for not importing internal/api's version.
func nodeDiskResourceID(nodeID string) string {
	return "node:" + nodeID
}

// EvaluateNodeDiskSpace runs one KindNodeDiskSpace rule against every
// node in nodes and returns its updated evaluation state plus, when
// firing, one human-readable notice line per node whose latest
// used-disk percentage meets or exceeds thresholdPercent, for Engine to
// attach to the outgoing Event.
//
// Firing follows EvaluatePatchStatus's shape, not EvaluateThreshold's:
// the instant any node is over threshold, with no ForDuration debounce.
// LastValue is the highest used-disk percentage seen across every node
// with a recent sample, not just the ones over threshold, matching
// EvaluatePatchStatus's own LastValue convention.
//
// A node missing either a disk_used_bytes or disk_total_bytes sample
// inside diskSpaceLookback (collector hasn't run yet), or reporting a
// total of zero or less, is skipped, neither confirming nor denying the
// condition. A node whose metrics query fails is logged and skipped
// too, the same "one broken resource must not block the rest" stance
// EvaluatePatchStatus already takes.
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
		resourceID := nodeDiskResourceID(node.ID)

		usedSamples, err := metrics.QueryMetrics(ctx, resourceID, telemetry.MetricDiskUsedBytes, from, now)
		if err != nil {
			logger.Warn("alerting: evaluate node_disk_space rule: query node disk usage failed, skipping node",
				slog.String("rule_id", r.ID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
			continue
		}
		totalSamples, err := metrics.QueryMetrics(ctx, resourceID, telemetry.MetricDiskTotalBytes, from, now)
		if err != nil {
			logger.Warn("alerting: evaluate node_disk_space rule: query node disk capacity failed, skipping node",
				slog.String("rule_id", r.ID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
			continue
		}
		if len(usedSamples) == 0 || len(totalSamples) == 0 {
			continue
		}

		total := totalSamples[len(totalSamples)-1].Value
		if total <= 0 {
			continue
		}
		used := usedSamples[len(usedSamples)-1].Value
		percent := used / total * 100

		if !haveMaxValue || percent > maxValue {
			maxValue, haveMaxValue = percent, true
		}

		if percent >= thresholdPercent {
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

func diskSpaceNotice(node store.Node, percent, threshold float64) string {
	name := node.Name
	if name == "" {
		name = node.ID
	}
	return fmt.Sprintf("%s: disk %.1f%% used (threshold %.1f%%)", name, percent, threshold)
}
