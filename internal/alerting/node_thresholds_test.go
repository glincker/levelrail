package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func cpuSource(now time.Time, byService map[string]float64) *perMetricSource {
	m := map[string]map[string][]telemetry.Sample{}
	for svc, v := range byService {
		m["service:"+svc] = map[string][]telemetry.Sample{nodeResourceUsageCPUMetric: {{Timestamp: now, Value: v}}}
	}
	return &perMetricSource{samples: m}
}

func TestEvaluateNodePressure_SustainedFiresOnceRecoversOnce(t *testing.T) {
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "build-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{"n1": {{Name: "web"}}}}
	policy := NodeThresholdPolicy{CPUPercent: 85, MemoryPercent: 90, SustainedIntervals: 3}
	r := Rule{ID: "r1", Kind: KindNodeCPU, Enabled: true}
	values := []float64{92, 92, 92, 93, 94, 40, 30}
	start := time.Now()
	var streaks map[string]int
	fired, recovered := 0, 0
	for i, v := range values {
		now := start.Add(time.Duration(i) * 30 * time.Second)
		next, _, s, err := EvaluateNodePressure(context.Background(), nodes, services, cpuSource(now, map[string]float64{"web": v}), r, policy, nil, streaks, now, nil)
		if err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		if i < 2 && next.Firing {
			t.Fatalf("tick %d fired before %d sustained intervals", i, policy.SustainedIntervals)
		}
		if !r.Firing && next.Firing {
			fired++
		}
		if r.Firing && !next.Firing {
			recovered++
		}
		r, streaks = next, s
	}
	if fired != 1 || recovered != 1 {
		t.Fatalf("fired=%d recovered=%d, want exactly one of each", fired, recovered)
	}
}

func TestEvaluateNodePressure_PerNodeOverrideSuppresses(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "build-1"}, {ID: "n2", Name: "edge-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{"n1": {{Name: "builder"}}, "n2": {{Name: "edge"}}}}
	high := 95
	overrides := map[string]store.NodeThresholdOverride{"n1": {NodeID: "n1", CPUPercent: &high}}
	policy := NodeThresholdPolicy{CPUPercent: 85, MemoryPercent: 90, SustainedIntervals: 1}
	r := Rule{ID: "r1", Kind: KindNodeCPU, Enabled: true}
	next, notices, _, err := EvaluateNodePressure(context.Background(), nodes, services,
		cpuSource(now, map[string]float64{"builder": 92, "edge": 92}), r, policy, overrides, nil, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Firing {
		t.Fatal("rule not firing, want edge-1 over the default 85")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "edge-1") {
		t.Fatalf("notices = %v, want only edge-1 (build-1 override 95 suppresses 92)", notices)
	}
}

func TestEvaluateNodePressure_MemoryUsesHostPercent(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "cp"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{"node:n1": {
		telemetry.MetricMemoryTotalBytes:     {{Timestamp: now, Value: 100}},
		telemetry.MetricMemoryAvailableBytes: {{Timestamp: now, Value: 5}},
	}}}
	r := Rule{ID: "r2", Kind: KindNodeMemory, Enabled: true}
	next, notices, _, err := EvaluateNodePressure(context.Background(), nodes, nil, metrics, r,
		NodeThresholdPolicy{CPUPercent: 85, MemoryPercent: 90, SustainedIntervals: 1}, nil, nil, now, nil)
	if err != nil || !next.Firing || len(notices) != 1 {
		t.Fatalf("firing=%v notices=%v err=%v, want 95%% memory to fire", next.Firing, notices, err)
	}
}
