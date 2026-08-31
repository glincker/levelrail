package alerting

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// fakeNodeServiceSource is an in-memory NodeServiceSource, the same
// hand-written-fake pattern fakeNodeSource (patch_status_test.go)
// already establishes for this package's tests.
type fakeNodeServiceSource struct {
	byNode map[string][]store.DesiredService
	err    error
}

func (f *fakeNodeServiceSource) ListDesiredServicesByNode(_ context.Context, nodeID string) ([]store.DesiredService, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byNode[nodeID], nil
}

func nodeResourceUsageRule() Rule {
	return Rule{ID: "r1", Name: "node load watch", Kind: KindNodeResourceUsage, Enabled: true}
}

func TestEvaluateNodeResourceUsage_NoNodes_NotFiring(t *testing.T) {
	got, notices, err := EvaluateNodeResourceUsage(context.Background(), &fakeNodeSource{}, &fakeNodeServiceSource{}, &perMetricSource{}, nodeResourceUsageRule(), 0, 0, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing with no nodes", got.Firing, notices)
	}
}

func TestEvaluateNodeResourceUsage_NodeWithNoPlacedServices_Skipped(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{}
	metrics := &perMetricSource{}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if got.Firing || len(notices) != 0 || got.LastValue != nil {
		t.Fatalf("got firing=%v notices=%v lastValue=%v, want not firing and no data for a node with nothing placed on it", got.Firing, notices, got.LastValue)
	}
}

func TestEvaluateNodeResourceUsage_AllNodesUnderThreshold_NotFiring(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}},
	}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"service:web": {
			nodeResourceUsageCPUMetric:    {{Timestamp: now, Value: 20}},
			nodeResourceUsageMemoryMetric: {{Timestamp: now, Value: 1 << 20}},
		},
	}}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, DefaultNodeMemoryThresholdBytes, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing when every node is under both thresholds", got.Firing, notices)
	}
	if got.LastValue == nil || *got.LastValue != 20 {
		t.Fatalf("LastValue = %v, want 20 (the highest summed CPU percent across nodes)", got.LastValue)
	}
}

func TestEvaluateNodeResourceUsage_CPUOverThreshold_Fires(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}},
		"n2": {{Name: "worker"}, {Name: "api"}},
	}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"service:web":    {nodeResourceUsageCPUMetric: {{Timestamp: now, Value: 10}}},
		"service:worker": {nodeResourceUsageCPUMetric: {{Timestamp: now, Value: 60}}},
		"service:api":    {nodeResourceUsageCPUMetric: {{Timestamp: now, Value: 40}}}, // n2 sums to 100
	}}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, DefaultNodeMemoryThresholdBytes, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if !got.Firing {
		t.Fatal("got.Firing = false, want true when a node's summed CPU crosses threshold")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "web-2") || !strings.Contains(notices[0], "CPU") {
		t.Fatalf("notices = %v, want exactly one mentioning web-2's CPU", notices)
	}
	if got.LastValue == nil || *got.LastValue != 100 {
		t.Fatalf("LastValue = %v, want 100 (n2's summed CPU)", got.LastValue)
	}
}

func TestEvaluateNodeResourceUsage_MemoryOverThreshold_Fires(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}},
	}}
	const gib = 1 << 30
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"service:web": {
			nodeResourceUsageCPUMetric:    {{Timestamp: now, Value: 5}},
			nodeResourceUsageMemoryMetric: {{Timestamp: now, Value: 5 * gib}},
		},
	}}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, 4*gib, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if !got.Firing {
		t.Fatal("got.Firing = false, want true when a node's summed memory crosses threshold")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "web-1") || !strings.Contains(notices[0], "memory") {
		t.Fatalf("notices = %v, want exactly one mentioning web-1's memory", notices)
	}
	// LastValue only ever tracks CPU, not memory: see EvaluateNodeResourceUsage's own doc comment.
	if got.LastValue == nil || *got.LastValue != 5 {
		t.Fatalf("LastValue = %v, want 5 (the CPU reading, even though memory is what fired)", got.LastValue)
	}
}

func TestEvaluateNodeResourceUsage_NoRecentSample_TreatedAsUnknownNotFiring(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}},
	}}
	metrics := &perMetricSource{}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if got.Firing || len(notices) != 0 || got.LastValue != nil {
		t.Fatalf("got firing=%v notices=%v lastValue=%v, want not firing and no data for a service with no sample", got.Firing, notices, got.LastValue)
	}
}

func TestEvaluateNodeResourceUsage_ZeroThresholds_FallBackToDefaults(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}},
	}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"service:web": {nodeResourceUsageCPUMetric: {{Timestamp: now, Value: 95}}},
	}}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 0, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v", err)
	}
	if !got.Firing || len(notices) != 1 {
		t.Fatalf("got firing=%v notices=%v, want firing at DefaultNodeCPUThresholdPercent (%g) when threshold is 0", got.Firing, notices, DefaultNodeCPUThresholdPercent)
	}
}

func TestEvaluateNodeResourceUsage_OneServiceQueryFails_StillSumsTheRest(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}, {Name: "worker"}},
	}}
	metrics := &perMetricSource{
		samples:       map[string]map[string][]telemetry.Sample{"service:worker": {nodeResourceUsageCPUMetric: {{Timestamp: now, Value: 90}}}},
		errByResource: map[string]error{"service:web": errors.New("query failed")},
	}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v, want nil (a single service's query failure must not fail the whole rule)", err)
	}
	if !got.Firing || len(notices) != 1 {
		t.Fatalf("got firing=%v notices=%v, want firing on worker's 90%% despite web's query failure", got.Firing, notices)
	}
}

func TestEvaluateNodeResourceUsage_OneNodeServiceListFails_SkipsThatNodeAndStillEvaluatesTheRest(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	services := &fakeNodeServiceSource{err: errors.New("list failed")}
	metrics := &perMetricSource{}

	got, notices, err := EvaluateNodeResourceUsage(context.Background(), nodes, services, metrics, nodeResourceUsageRule(), 80, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeResourceUsage() error = %v, want nil (a node's placement-list failure must not fail the whole rule)", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing when every node's placement list fails", got.Firing, notices)
	}
}

func TestEngine_Tick_NodeResourceUsageFires_NotifiesOnceThenDoesNotDoubleFire(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "web"}},
	}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"service:web": {nodeResourceUsageCPUMetric: {{Timestamp: now, Value: 95}}},
	}}

	r := nodeResourceUsageRule()
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := NewEngine(rules, metrics, nil, nil, nil, nil, 0, 0, nodes, 0, 0, services, 80, 0, func(Rule) Notifier { return spy }, nil)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("after first Tick, notify calls = %d, want 1", len(calls))
	}
	if len(calls[0].ResourceUsageNotices) != 1 || !strings.Contains(calls[0].ResourceUsageNotices[0], "web-1") {
		t.Fatalf("ResourceUsageNotices = %v, want exactly one entry mentioning web-1", calls[0].ResourceUsageNotices)
	}

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 1 {
		t.Fatalf("after second Tick with the same firing state, notify calls = %d, want still 1 (no double-fire)", len(calls))
	}
}
