package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// NodeThresholdPolicy is the live platform default for node pressure rules, read from runtime settings each tick.
type NodeThresholdPolicy struct {
	CPUPercent         float64
	MemoryPercent      float64
	DiskPercent        float64
	SustainedIntervals int
}

// NodeThresholdOverrideSource lists per-node overrides; *store.DB satisfies it.
type NodeThresholdOverrideSource interface {
	ListNodeThresholdOverrides(ctx context.Context) (map[string]store.NodeThresholdOverride, error)
}

// nodeStreaks counts consecutive breaching evaluations per rule and node, so one spike never fires.
type nodeStreaks struct {
	mu sync.Mutex
	m  map[string]map[string]int
}

func (s *nodeStreaks) get(ruleID string) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[ruleID]
}

func (s *nodeStreaks) put(ruleID string, v map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]map[string]int{}
	}
	s.m[ruleID] = v
}

// SetNodeThresholds enables node_cpu and node_memory rules and makes node_disk_space use the live default and overrides.
func (e *Engine) SetNodeThresholds(policy func() NodeThresholdPolicy, overrides NodeThresholdOverrideSource) {
	e.nodePolicy = policy
	e.nodeOverrides = overrides
}

func (e *Engine) loadNodeOverrides(ctx context.Context) map[string]store.NodeThresholdOverride {
	if e.nodeOverrides == nil {
		return nil
	}
	m, err := e.nodeOverrides.ListNodeThresholdOverrides(ctx)
	if err != nil {
		e.logger.Warn("alerting: list node threshold overrides failed, using defaults", slog.String("error", err.Error()))
		return nil
	}
	return m
}

// nodePressureLimit picks the node override when set, else the platform default for the rule's kind.
func nodePressureLimit(kind Kind, policy NodeThresholdPolicy, o store.NodeThresholdOverride) float64 {
	if kind == KindNodeCPU {
		if o.CPUPercent != nil {
			return float64(*o.CPUPercent)
		}
		return policy.CPUPercent
	}
	if o.MemoryPercent != nil {
		return float64(*o.MemoryPercent)
	}
	return policy.MemoryPercent
}

// EvaluateNodePressure runs a node_cpu or node_memory rule. A node counts only after SustainedIntervals breaching
// evaluations in a row; prev is the per-node streak from the last tick and the returned map replaces it.
func EvaluateNodePressure(ctx context.Context, nodes NodeSource, services NodeServiceSource, metrics MetricsSource, r Rule, policy NodeThresholdPolicy, overrides map[string]store.NodeThresholdOverride, prev map[string]int, now time.Time, logger *slog.Logger) (Rule, []string, map[string]int, error) {
	if logger == nil {
		logger = slog.Default()
	}
	all, err := nodes.ListNodes(ctx)
	if err != nil {
		return r, nil, prev, fmt.Errorf("alerting: evaluate rule %q: list nodes: %w", r.ID, err)
	}
	need := policy.SustainedIntervals
	if need < 1 {
		need = 1
	}
	next := r
	next.LastEvaluatedAt = &now
	from := now.Add(-nodeResourceUsageLookback)
	streaks := make(map[string]int, len(all))

	var (
		notices  []string
		breached bool
		maxValue float64
		haveMax  bool
	)
	for _, node := range all {
		value, have := nodePressureValue(ctx, services, metrics, r, node, from, now, logger)
		if !have {
			continue
		}
		if !haveMax || value > maxValue {
			maxValue, haveMax = value, true
		}
		limit := nodePressureLimit(r.Kind, policy, overrides[node.ID])
		if limit <= 0 || value < limit {
			continue
		}
		streaks[node.ID] = prev[node.ID] + 1
		// A firing rule after a restart has no streaks; a node still over stays counted so it does not recover and refire.
		if streaks[node.ID] >= need || r.Firing {
			breached = true
			notices = append(notices, nodePressureNotice(r.Kind, node, value, limit))
		}
	}
	if haveMax {
		v := maxValue
		next.LastValue = &v
	}
	return advanceState(next, r, breached, 0, now), notices, streaks, nil
}

// nodePressureValue is summed service CPU for node_cpu, or used host memory percent for node_memory.
func nodePressureValue(ctx context.Context, services NodeServiceSource, metrics MetricsSource, r Rule, node store.Node, from, now time.Time, logger *slog.Logger) (float64, bool) {
	if r.Kind == KindNodeCPU {
		if services == nil {
			return 0, false
		}
		placed, err := services.ListDesiredServicesByNode(ctx, node.ID)
		if err != nil || len(placed) == 0 {
			return 0, false
		}
		return sumLatestServiceMetric(ctx, metrics, placed, nodeResourceUsageCPUMetric, from, now, r.ID, node.ID, logger)
	}
	total, okTotal := latestNodeSample(ctx, metrics, node.ID, telemetry.MetricMemoryTotalBytes, from, now)
	avail, okAvail := latestNodeSample(ctx, metrics, node.ID, telemetry.MetricMemoryAvailableBytes, from, now)
	if !okTotal || !okAvail || total <= 0 {
		return 0, false
	}
	return (total - avail) / total * 100, true
}

func latestNodeSample(ctx context.Context, metrics MetricsSource, nodeID, metric string, from, now time.Time) (float64, bool) {
	samples, err := metrics.QueryMetrics(ctx, nodeDiskResourceID(nodeID), metric, from, now)
	if err != nil || len(samples) == 0 {
		return 0, false
	}
	return samples[len(samples)-1].Value, true
}

func nodePressureNotice(kind Kind, node store.Node, value, limit float64) string {
	name := node.Name
	if name == "" {
		name = node.ID
	}
	what := "memory"
	if kind == KindNodeCPU {
		what = "CPU"
	}
	return fmt.Sprintf("%s: %s %.1f%% (threshold %.0f%%)", name, what, value, limit)
}
