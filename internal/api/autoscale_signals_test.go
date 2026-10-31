package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type seriesTelemetry struct {
	series map[string][]telemetry.Sample
}

func (s seriesTelemetry) QueryMetrics(_ context.Context, _ string, metric string, _, _ time.Time) ([]telemetry.Sample, error) {
	return s.series[metric], nil
}

func (seriesTelemetry) QueryLogs(context.Context, string, time.Time, time.Time, string) ([]telemetry.LogEntry, error) {
	return nil, nil
}

func (seriesTelemetry) LatestByMetric(context.Context, string) ([]telemetry.Sample, error) {
	return nil, nil
}

func sample(metric string, at time.Time, v float64) telemetry.Sample {
	return telemetry.Sample{ResourceID: "service:web", Metric: metric, Timestamp: at, Value: v}
}

func TestAutoscaleSignalsMetric(t *testing.T) {
	t.Parallel()
	now := time.Now()
	fresh, old := now.Add(-10*time.Second), now.Add(-30*time.Minute)
	gib := int64(1 << 30)
	tests := []struct {
		name      string
		metric    string
		res       *store.ServiceResources
		series    map[string][]telemetry.Sample
		wantKnown bool
		wantValue float64
	}{
		{"cpu normalizes by the limit", autoscale.MetricCPUPercent, &store.ServiceResources{NanoCPUs: 2e9},
			map[string][]telemetry.Sample{"cpu_percent": {sample("cpu_percent", fresh, 100), sample("cpu_percent", fresh, 140)}}, true, 60},
		{"cpu without a limit is percent of one core", autoscale.MetricCPUPercent, nil,
			map[string][]telemetry.Sample{"cpu_percent": {sample("cpu_percent", fresh, 80)}}, true, 80},
		{"cpu with no samples is unknown", autoscale.MetricCPUPercent, nil, nil, false, 0},
		{"cpu with stale samples is unknown", autoscale.MetricCPUPercent, nil,
			map[string][]telemetry.Sample{"cpu_percent": {sample("cpu_percent", old, 90)}}, false, 0},
		{"memory against the configured limit", autoscale.MetricMemoryPercent, &store.ServiceResources{MemoryBytes: gib},
			map[string][]telemetry.Sample{"memory_usage_bytes": {sample("memory_usage_bytes", fresh, float64(gib)/2)}}, true, 50},
		{"memory falls back to the reported limit", autoscale.MetricMemoryPercent, nil,
			map[string][]telemetry.Sample{
				"memory_usage_bytes": {sample("memory_usage_bytes", fresh, float64(gib)/4)},
				"memory_limit_bytes": {sample("memory_limit_bytes", fresh, float64(gib))},
			}, true, 25},
		{"memory with no limit anywhere is unknown", autoscale.MetricMemoryPercent, nil,
			map[string][]telemetry.Sample{"memory_usage_bytes": {sample("memory_usage_bytes", fresh, 100)}}, false, 0},
		{"silence with live containers reads as idle", autoscale.MetricRequestsPerReplica, nil,
			map[string][]telemetry.Sample{"cpu_percent": {sample("cpu_percent", fresh, 1)}}, true, 0},
		{"silence with no container samples is unknown", autoscale.MetricRequestsPerReplica, nil, nil, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Resources: tc.res}); err != nil {
				t.Fatal(err)
			}
			rt.telemetry = seriesTelemetry{series: tc.series}
			pol := autoscale.Policy{App: "web", Metric: tc.metric, Target: 50}
			got, err := autoscaleSignals{rt: rt}.Metric(context.Background(), pol, 2, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Known != tc.wantKnown || (tc.wantKnown && got.Value != tc.wantValue) {
				t.Fatalf("reading = %+v, want known=%v value=%v", got, tc.wantKnown, tc.wantValue)
			}
			if !got.Known && got.Note == "" {
				t.Fatal("an unknown reading must say why")
			}
		})
	}
}

func TestAutoscaleSignalsWithoutTelemetryIsUnknown(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	rt.telemetry = nil
	got, err := autoscaleSignals{rt: rt}.Metric(context.Background(), autoscale.Policy{App: "web", Metric: autoscale.MetricCPUPercent}, 1, time.Now())
	if err != nil || got.Known || got.Note == "" {
		t.Fatalf("reading = %+v err = %v", got, err)
	}
}

func TestAutoscaleSignalsHeadroomIsUnknownWithoutMultiNode(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	if n, err := (autoscaleSignals{rt: rt}).Headroom(context.Background(), "web", 3); err != nil || n >= 0 {
		t.Fatalf("headroom = %d err = %v, want unknown", n, err)
	}
}
