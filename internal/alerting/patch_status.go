package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DefaultPatchStatusThreshold is the pending security patch count that fires a patch_status rule; 1 means any pending patch.
const DefaultPatchStatusThreshold = 1

// patchStatusLookback matches internal/api's osPatchLookback, wide enough to survive a slow or restarted collector.
const patchStatusLookback = 48 * time.Hour

// NodeSource is the narrow node-listing surface EvaluatePatchStatus
// needs. *store.DB satisfies this structurally, the same as CertSource
// above.
type NodeSource interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
}

// nodePatchResourceID duplicates internal/api's nodeResourceID to avoid an import cycle.
func nodePatchResourceID(nodeID string) string {
	return "node:" + nodeID
}

// EvaluatePatchStatus evaluates one patch_status rule across nodes; firing is immediate, with no debounce, since the collector samples hourly.
// Nodes with no recent sample are skipped, neither confirming nor denying the condition.
func EvaluatePatchStatus(ctx context.Context, nodes NodeSource, metrics MetricsSource, r Rule, threshold float64, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if threshold <= 0 {
		threshold = DefaultPatchStatusThreshold
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

	from := now.Add(-patchStatusLookback)

	var (
		notices      []string
		anyUnhealthy bool
		maxValue     float64
		haveMaxValue bool
	)

	for _, node := range all {
		value, have, firing := evaluatePatchStatusForNode(ctx, metrics, r.ID, node, threshold, from, now, logger)
		if !have {
			continue
		}

		if !haveMaxValue || value > maxValue {
			maxValue, haveMaxValue = value, true
		}

		if firing {
			anyUnhealthy = true
			notices = append(notices, patchStatusNotice(node, value, threshold))
		}
	}

	if haveMaxValue {
		v := maxValue
		next.LastValue = &v
	}

	return advanceState(next, r, anyUnhealthy, 0, now), notices, nil
}

// evaluatePatchStatusForNode lets node_alert_status.go run the same check on demand, without waiting for the next tick.
func evaluatePatchStatusForNode(ctx context.Context, metrics MetricsSource, ruleID string, node store.Node, threshold float64, from, now time.Time, logger *slog.Logger) (value float64, have, firing bool) {
	samples, err := metrics.QueryMetrics(ctx, nodePatchResourceID(node.ID), telemetry.MetricOSSecurityPatchesAvailable, from, now)
	if err != nil {
		logger.Warn("alerting: evaluate patch_status rule: query node patch status failed, skipping node",
			slog.String("rule_id", ruleID), slog.String("node_id", node.ID), slog.String("error", err.Error()))
		return 0, false, false
	}
	if len(samples) == 0 {
		return 0, false, false
	}
	value = samples[len(samples)-1].Value
	return value, true, value >= threshold
}

func patchStatusNotice(node store.Node, value, threshold float64) string {
	name := node.Name
	if name == "" {
		name = node.ID
	}
	return fmt.Sprintf("%s: %g security patches pending (threshold %g)", name, value, threshold)
}
