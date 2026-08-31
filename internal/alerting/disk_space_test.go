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

// perMetricSource is a NodeSource-paired MetricsSource keyed by
// resourceID and metric name, unlike patch_status_test.go's
// perResourceMetricsSource (which ignores the metric argument entirely):
// EvaluateNodeDiskSpace queries two different metrics per node, so a
// useful test needs to tell disk_used_bytes and disk_total_bytes apart.
type perMetricSource struct {
	samples       map[string]map[string][]telemetry.Sample
	errByResource map[string]error
}

func (f *perMetricSource) QueryMetrics(_ context.Context, resourceID, metric string, _, _ time.Time) ([]telemetry.Sample, error) {
	if err, ok := f.errByResource[resourceID]; ok {
		return nil, err
	}
	return f.samples[resourceID][metric], nil
}

func diskSpaceRule() Rule {
	return Rule{ID: "r1", Name: "disk watch", Kind: KindNodeDiskSpace, Enabled: true}
}

func diskSamples(usedBytes, totalBytes float64, ts time.Time) map[string][]telemetry.Sample {
	return map[string][]telemetry.Sample{
		telemetry.MetricDiskUsedBytes:  {{Timestamp: ts, Value: usedBytes}},
		telemetry.MetricDiskTotalBytes: {{Timestamp: ts, Value: totalBytes}},
	}
}

func TestEvaluateNodeDiskSpace_NoNodes_NotFiring(t *testing.T) {
	got, notices, err := EvaluateNodeDiskSpace(context.Background(), &fakeNodeSource{}, &perMetricSource{}, diskSpaceRule(), 0, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing with no nodes", got.Firing, notices)
	}
}

func TestEvaluateNodeDiskSpace_AllNodesUnderThreshold_NotFiring(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"node:n1": diskSamples(10, 100, now), // 10%
		"node:n2": diskSamples(20, 100, now), // 20%
	}}

	got, notices, err := EvaluateNodeDiskSpace(context.Background(), nodes, metrics, diskSpaceRule(), 90, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing when every node is under threshold", got.Firing, notices)
	}
	if got.LastValue == nil || *got.LastValue != 20 {
		t.Fatalf("LastValue = %v, want 20 (the highest reading across nodes)", got.LastValue)
	}
}

func TestEvaluateNodeDiskSpace_OneNodeOverThreshold_Fires(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"node:n1": diskSamples(10, 100, now), // 10%
		"node:n2": diskSamples(95, 100, now), // 95%
	}}

	got, notices, err := EvaluateNodeDiskSpace(context.Background(), nodes, metrics, diskSpaceRule(), 90, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v", err)
	}
	if !got.Firing {
		t.Fatal("got.Firing = false, want true when one node is over threshold")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "web-2") {
		t.Fatalf("notices = %v, want exactly one mentioning web-2", notices)
	}
	if got.LastValue == nil || *got.LastValue != 95 {
		t.Fatalf("LastValue = %v, want 95 (the highest reading across nodes)", got.LastValue)
	}
}

func TestEvaluateNodeDiskSpace_NoRecentSample_TreatedAsUnknownNotFiring(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{}}

	got, notices, err := EvaluateNodeDiskSpace(context.Background(), nodes, metrics, diskSpaceRule(), 90, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v", err)
	}
	if got.Firing || len(notices) != 0 || got.LastValue != nil {
		t.Fatalf("got firing=%v notices=%v lastValue=%v, want not firing and no data for a node with no sample", got.Firing, notices, got.LastValue)
	}
}

func TestEvaluateNodeDiskSpace_ZeroTotal_SkipsNode(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"node:n1": diskSamples(0, 0, now),
	}}

	got, notices, err := EvaluateNodeDiskSpace(context.Background(), nodes, metrics, diskSpaceRule(), 90, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v", err)
	}
	if got.Firing || len(notices) != 0 || got.LastValue != nil {
		t.Fatalf("got firing=%v notices=%v lastValue=%v, want not firing and no data for a node reporting zero total", got.Firing, notices, got.LastValue)
	}
}

func TestEvaluateNodeDiskSpace_ZeroThreshold_FallsBackToDefault(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"node:n1": diskSamples(95, 100, now), // 95%
	}}

	got, notices, err := EvaluateNodeDiskSpace(context.Background(), nodes, metrics, diskSpaceRule(), 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v", err)
	}
	if !got.Firing || len(notices) != 1 {
		t.Fatalf("got firing=%v notices=%v, want firing at DefaultNodeDiskSpaceThresholdPercent (%g) when threshold is 0", got.Firing, notices, DefaultNodeDiskSpaceThresholdPercent)
	}
}

func TestEvaluateNodeDiskSpace_OneNodeQueryFails_SkipsThatNodeAndStillEvaluatesTheRest(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	metrics := &perMetricSource{
		samples:       map[string]map[string][]telemetry.Sample{"node:n2": diskSamples(95, 100, now)},
		errByResource: map[string]error{"node:n1": errors.New("query failed")},
	}

	got, notices, err := EvaluateNodeDiskSpace(context.Background(), nodes, metrics, diskSpaceRule(), 90, now, nil)
	if err != nil {
		t.Fatalf("EvaluateNodeDiskSpace() error = %v, want nil (a single node's query failure must not fail the whole rule)", err)
	}
	if !got.Firing || len(notices) != 1 || !strings.Contains(notices[0], "web-2") {
		t.Fatalf("got firing=%v notices=%v, want firing on web-2 despite web-1's query failure", got.Firing, notices)
	}
}

func TestEngine_Tick_NodeDiskSpaceFires_NotifiesOnceThenDoesNotDoubleFire(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perMetricSource{samples: map[string]map[string][]telemetry.Sample{
		"node:n1": diskSamples(95, 100, now),
	}}

	r := diskSpaceRule()
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := NewEngine(rules, metrics, nil, nil, nil, nil, 0, 0, nodes, 0, 90, func(Rule) Notifier { return spy }, nil)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("after first Tick, notify calls = %d, want 1", len(calls))
	}
	if len(calls[0].DiskSpaceNotices) != 1 || !strings.Contains(calls[0].DiskSpaceNotices[0], "web-1") {
		t.Fatalf("DiskSpaceNotices = %v, want exactly one entry mentioning web-1", calls[0].DiskSpaceNotices)
	}

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 1 {
		t.Fatalf("after second Tick with the same firing state, notify calls = %d, want still 1 (no double-fire)", len(calls))
	}
}
