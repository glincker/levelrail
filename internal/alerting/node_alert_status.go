package alerting

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// NodeAlertState is one node-scoped alert kind's live status for one
// specific node, computed on demand by CheckNodeAlertStatus rather than
// read from a rule's stored, all-nodes-aggregated LastValue.
type NodeAlertState string

// The three states CheckNodeAlertStatus can report per kind.
// NodeAlertUnknown is distinct from NodeAlertOK on purpose: a node with no
// recent sample must never be reported as healthy, the same "don't
// silently confirm what you can't actually see" stance every evaluator in
// this package already takes on a per-node basis.
const (
	NodeAlertOK      NodeAlertState = "ok"
	NodeAlertFiring  NodeAlertState = "firing"
	NodeAlertUnknown NodeAlertState = "unknown"
)

// NodeAlertStatus is one node's live status across the three node-scoped,
// platform-wide alert kinds.
type NodeAlertStatus struct {
	PatchStatus       NodeAlertState
	NodeDiskSpace     NodeAlertState
	NodeResourceUsage NodeAlertState
}

// liveNodeAlertCheckLabel is the rule_id logged by a live check's per-node
// helper calls, so a warning from CheckNodeAlertStatus reads distinctly
// from one logged during Engine's own scheduled tick.
const liveNodeAlertCheckLabel = "on-demand-node-check"

// CheckNodeAlertStatus live-evaluates patch_status, node_disk_space, and
// node_resource_usage for node, right now, rather than waiting for
// Engine's next tick or reading a rule's stored aggregate LastValue
// (which only ever holds the worst value seen across every node, not
// which node it came from). It calls each evaluator's own per-node check
// helper directly so this can never drift from what a real rule would
// decide. A threshold of 0 falls back to that kind's own default, the
// same convention EvaluatePatchStatus/EvaluateNodeDiskSpace/
// EvaluateNodeResourceUsage already use.
func CheckNodeAlertStatus(ctx context.Context, node store.Node, services NodeServiceSource, metrics MetricsSource,
	patchThreshold, diskThresholdPercent, cpuThresholdPercent, memoryThresholdBytes float64, now time.Time, logger *slog.Logger) NodeAlertStatus {
	if logger == nil {
		logger = slog.Default()
	}
	if patchThreshold <= 0 {
		patchThreshold = DefaultPatchStatusThreshold
	}
	if diskThresholdPercent <= 0 {
		diskThresholdPercent = DefaultNodeDiskSpaceThresholdPercent
	}
	if cpuThresholdPercent <= 0 {
		cpuThresholdPercent = DefaultNodeCPUThresholdPercent
	}
	if memoryThresholdBytes <= 0 {
		memoryThresholdBytes = DefaultNodeMemoryThresholdBytes
	}

	_, havePatch, firingPatch := evaluatePatchStatusForNode(
		ctx, metrics, liveNodeAlertCheckLabel, node, patchThreshold, now.Add(-patchStatusLookback), now, logger)

	_, haveDisk, firingDisk := evaluateNodeDiskSpaceForNode(
		ctx, metrics, liveNodeAlertCheckLabel, node, diskThresholdPercent, now.Add(-diskSpaceLookback), now, logger)

	_, _, haveCPU, haveMem, overCPU, overMem := evaluateNodeResourceUsageForNode(
		ctx, services, metrics, liveNodeAlertCheckLabel, node, cpuThresholdPercent, memoryThresholdBytes,
		now.Add(-nodeResourceUsageLookback), now, logger)

	return NodeAlertStatus{
		PatchStatus:       nodeAlertState(havePatch, firingPatch),
		NodeDiskSpace:     nodeAlertState(haveDisk, firingDisk),
		NodeResourceUsage: nodeAlertState(haveCPU || haveMem, overCPU || overMem),
	}
}

func nodeAlertState(have, firing bool) NodeAlertState {
	if !have {
		return NodeAlertUnknown
	}
	if firing {
		return NodeAlertFiring
	}
	return NodeAlertOK
}
