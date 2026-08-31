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

// fakeNodeSource is an in-memory NodeSource, the same hand-written-fake
// pattern fakeCertSource (cert_expiry_test.go) already establishes for
// this package's tests.
type fakeNodeSource struct {
	nodes []store.Node
	err   error
}

func (f *fakeNodeSource) ListNodes(_ context.Context) ([]store.Node, error) {
	return f.nodes, f.err
}

// perResourceMetricsSource returns samples keyed by resourceID, unlike
// evaluate_test.go's fakeMetricsSource (which ignores its resourceID
// argument entirely): EvaluatePatchStatus queries once per node, so a
// useful test needs different nodes to see different values.
type perResourceMetricsSource struct {
	samples       map[string][]telemetry.Sample
	err           error
	errByResource map[string]error
}

func (f *perResourceMetricsSource) QueryMetrics(_ context.Context, resourceID, _ string, _, _ time.Time) ([]telemetry.Sample, error) {
	if f.err != nil {
		return nil, f.err
	}
	if err, ok := f.errByResource[resourceID]; ok {
		return nil, err
	}
	return f.samples[resourceID], nil
}

func patchStatusRule() Rule {
	return Rule{ID: "r1", Name: "patch watch", Kind: KindPatchStatus, Enabled: true}
}

func TestEvaluatePatchStatus_NoNodes_NotFiring(t *testing.T) {
	got, notices, err := EvaluatePatchStatus(context.Background(), &fakeNodeSource{}, &perResourceMetricsSource{}, patchStatusRule(), 0, time.Now(), nil)
	if err != nil {
		t.Fatalf("EvaluatePatchStatus() error = %v", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing with no nodes", got.Firing, notices)
	}
}

func TestEvaluatePatchStatus_AllNodesUnderThreshold_NotFiring(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	metrics := &perResourceMetricsSource{samples: map[string][]telemetry.Sample{
		"node:n1": {{Timestamp: now, Value: 0}},
		"node:n2": {{Timestamp: now, Value: 0}},
	}}

	got, notices, err := EvaluatePatchStatus(context.Background(), nodes, metrics, patchStatusRule(), 1, now, nil)
	if err != nil {
		t.Fatalf("EvaluatePatchStatus() error = %v", err)
	}
	if got.Firing || len(notices) != 0 {
		t.Fatalf("got firing=%v notices=%v, want not firing when every node is under threshold", got.Firing, notices)
	}
	if got.LastValue == nil || *got.LastValue != 0 {
		t.Fatalf("LastValue = %v, want 0 (the highest reading across nodes)", got.LastValue)
	}
}

func TestEvaluatePatchStatus_OneNodeOverThreshold_Fires(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	metrics := &perResourceMetricsSource{samples: map[string][]telemetry.Sample{
		"node:n1": {{Timestamp: now, Value: 0}},
		"node:n2": {{Timestamp: now, Value: 3}},
	}}

	got, notices, err := EvaluatePatchStatus(context.Background(), nodes, metrics, patchStatusRule(), 1, now, nil)
	if err != nil {
		t.Fatalf("EvaluatePatchStatus() error = %v", err)
	}
	if !got.Firing {
		t.Fatal("got.Firing = false, want true when one node is over threshold")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "web-2") {
		t.Fatalf("notices = %v, want exactly one mentioning web-2", notices)
	}
	if got.LastValue == nil || *got.LastValue != 3 {
		t.Fatalf("LastValue = %v, want 3 (the highest reading across nodes)", got.LastValue)
	}
}

func TestEvaluatePatchStatus_NoRecentSample_TreatedAsUnknownNotFiring(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perResourceMetricsSource{samples: map[string][]telemetry.Sample{}}

	got, notices, err := EvaluatePatchStatus(context.Background(), nodes, metrics, patchStatusRule(), 1, now, nil)
	if err != nil {
		t.Fatalf("EvaluatePatchStatus() error = %v", err)
	}
	if got.Firing || len(notices) != 0 || got.LastValue != nil {
		t.Fatalf("got firing=%v notices=%v lastValue=%v, want not firing and no data for a node with no sample", got.Firing, notices, got.LastValue)
	}
}

func TestEvaluatePatchStatus_ZeroThreshold_FallsBackToDefault(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perResourceMetricsSource{samples: map[string][]telemetry.Sample{
		"node:n1": {{Timestamp: now, Value: 1}},
	}}

	got, notices, err := EvaluatePatchStatus(context.Background(), nodes, metrics, patchStatusRule(), 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluatePatchStatus() error = %v", err)
	}
	if !got.Firing || len(notices) != 1 {
		t.Fatalf("got firing=%v notices=%v, want firing at DefaultPatchStatusThreshold (%g) when threshold is 0", got.Firing, notices, float64(DefaultPatchStatusThreshold))
	}
}

func TestEvaluatePatchStatus_OneNodeQueryFails_SkipsThatNodeAndStillEvaluatesTheRest(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}, {ID: "n2", Name: "web-2"}}}
	metrics := &perResourceMetricsSource{
		samples:       map[string][]telemetry.Sample{"node:n2": {{Timestamp: now, Value: 5}}},
		errByResource: map[string]error{"node:n1": errors.New("query failed")},
	}

	got, notices, err := EvaluatePatchStatus(context.Background(), nodes, metrics, patchStatusRule(), 1, now, nil)
	if err != nil {
		t.Fatalf("EvaluatePatchStatus() error = %v, want nil (a single node's query failure must not fail the whole rule)", err)
	}
	if !got.Firing || len(notices) != 1 || !strings.Contains(notices[0], "web-2") {
		t.Fatalf("got firing=%v notices=%v, want firing on web-2 despite web-1's query failure", got.Firing, notices)
	}
}

func TestEngine_Tick_PatchStatusFires_NotifiesOnceThenDoesNotDoubleFire(t *testing.T) {
	now := time.Now()
	nodes := &fakeNodeSource{nodes: []store.Node{{ID: "n1", Name: "web-1"}}}
	metrics := &perResourceMetricsSource{samples: map[string][]telemetry.Sample{
		"node:n1": {{Timestamp: now, Value: 5}},
	}}

	r := patchStatusRule()
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := NewEngine(rules, metrics, nil, nil, nil, nil, 0, 0, nodes, 1, 0, nil, 0, 0, func(Rule) Notifier { return spy }, nil)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	calls := spy.calls()
	if len(calls) != 1 {
		t.Fatalf("after first Tick, notify calls = %d, want 1", len(calls))
	}
	if len(calls[0].PatchNotices) != 1 || !strings.Contains(calls[0].PatchNotices[0], "web-1") {
		t.Fatalf("PatchNotices = %v, want exactly one entry mentioning web-1", calls[0].PatchNotices)
	}

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 1 {
		t.Fatalf("after second Tick with the same firing state, notify calls = %d, want still 1 (no double-fire)", len(calls))
	}
}
