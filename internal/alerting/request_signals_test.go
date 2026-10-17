package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type requestFake struct{ byMetric map[string]float64 }

func (f requestFake) QueryMetrics(_ context.Context, _, metric string, from, _ time.Time) ([]telemetry.Sample, error) {
	v, ok := f.byMetric[metric]
	if !ok {
		return nil, nil
	}
	return []telemetry.Sample{{Metric: metric, Timestamp: from.Add(time.Second), Value: v}}, nil
}

func TestEvaluateThresholdRequestSignals(t *testing.T) {
	now := time.Now().UTC()
	traffic := requestFake{byMetric: map[string]float64{
		telemetry.MetricHTTPRequests:     100,
		telemetry.MetricHTTPResponses5xx: 10,
		telemetry.LatencyBucketMetric(6): 100,
	}}
	cases := []struct {
		name   string
		src    requestFake
		metric string
		thresh float64
		fire   bool
	}{
		{"error rate above", traffic, MetricRequestErrorRate5xxPct, 5, true},
		{"error rate below", traffic, MetricRequestErrorRate5xxPct, 20, false},
		{"p99 above", traffic, MetricRequestP99Ms, 400, true},
		{"p99 below", traffic, MetricRequestP99Ms, 600, false},
		{"no traffic never fires", requestFake{}, MetricRequestErrorRate5xxPct, 0, false},
	}
	for _, c := range cases {
		r := Rule{ID: "r", ResourceID: "service:web", Kind: KindThreshold, Metric: c.metric, Comparator: GreaterThan, Threshold: c.thresh}
		got, err := EvaluateThreshold(context.Background(), c.src, r, now)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Firing != c.fire {
			t.Errorf("%s: firing = %v, want %v", c.name, got.Firing, c.fire)
		}
	}
}

func TestRequestSignalsAreRequestPath(t *testing.T) {
	for _, m := range []string{MetricRequestErrorRate5xxPct, MetricRequestP95Ms, MetricRequestP99Ms} {
		if !IsRequestSignal(m) || ClassifyMetric(m) != MetricClassRequestPath {
			t.Errorf("%s not classified as request path signal", m)
		}
	}
}
