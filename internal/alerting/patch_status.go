package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DefaultPatchStatusThreshold is how many pending security patches a
// node can have before a kind=patch_status rule fires, when no override
// is configured. hostpatch.go only ever reports a point-in-time count,
// never a per-package age, so count is the only signal this kind can
// evaluate; 1 means "any security patch pending at all," the same
// zero-tolerance default GET /api/v1/nodes/{id}/patch-status's dashboard
// card already implies by showing Security > 0 as a concern.
const DefaultPatchStatusThreshold = 1

// patchStatusLookback mirrors internal/api's osPatchLookback exactly:
// wide enough to survive a slow or just-restarted HostPatchCollector
// (its own default interval is an hour) without losing the last real
// reading.
const patchStatusLookback = 48 * time.Hour

// NodeSource is the narrow node-listing surface EvaluatePatchStatus
// needs. *store.DB satisfies this structurally, the same as CertSource
// above.
type NodeSource interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
}

// nodePatchResourceID duplicates internal/api's own nodeResourceID
// rather than importing it: internal/api imports internal/alerting (for
// AlertRules et al.), so the reverse import would cycle, the same
// reasoning CertSource's own doc comment gives for duplicating its
// method set.
func nodePatchResourceID(nodeID string) string {
	return "node:" + nodeID
}

// EvaluatePatchStatus runs one KindPatchStatus rule against every node
// in nodes and returns its updated evaluation state plus, when firing,
// one human-readable notice line per node whose latest security-patch
// count meets or exceeds threshold, for Engine to attach to the
// outgoing Event.
//
// Firing follows EvaluateCertExpiry's shape, not EvaluateThreshold's:
// the instant any node is over threshold, with no ForDuration debounce.
// HostPatchCollector already only samples once an hour, so there is
// nothing left for a per-tick debounce to smooth out. LastValue is the
// highest security-patch count seen across every node with a recent
// sample, not just the ones over threshold, so an operator can see "how
// close" the fleet is even while every node is healthy, matching
// EvaluateCertExpiry's own LastValue convention.
//
// A node with no sample inside patchStatusLookback (no supported
// package manager, or the collector hasn't run yet) is skipped, neither
// confirming nor denying the condition, the same "no recent data" stance
// EvaluateThreshold takes. A node whose own metrics query fails is
// logged and skipped too, rather than failing the whole rule: the same
// "one broken resource must not block the rest" stance
// ListCertificates takes on a single malformed certificate.
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

// evaluatePatchStatusForNode is one node's own check within
// EvaluatePatchStatus's loop, pulled out so node_alert_status.go's live,
// single-node check can call the identical logic without waiting for
// Engine's next tick. have is false when there is no recent sample to
// judge, matching the loop's own "skip, don't confirm or deny" stance.
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
