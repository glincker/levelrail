package alerting

import (
	"context"
	"testing"
	"time"
)

func TestClassifyMetric(t *testing.T) {
	tests := []struct {
		metric string
		want   MetricClass
	}{
		{"http_requests", MetricClassRequestPath},
		{"http_responses_5xx", MetricClassRequestPath},
		{"http_responses_4xx", MetricClassRequestPath},
		{"http_upstream_errors", MetricClassRequestPath},
		{"http_bytes_out", MetricClassRequestPath},
		{"http_latency_bucket_250", MetricClassRequestPath},
		{"http_latency_bucket_inf", MetricClassRequestPath},
		{"lb_upstreams_healthy", MetricClassRequestPath},
		{"lb_active_requests", MetricClassRequestPath},
		{"cpu_percent", MetricClassResource},
		{"autoscale_no_capacity", MetricClassResource},
		{"autoscale_flaps_1h", MetricClassResource},
		{"memory_usage_bytes", MetricClassResource},
		{"network_rx_bytes", MetricClassResource},
		{"disk_write_bytes", MetricClassResource},
		{"disk_used_bytes", MetricClassResource},
		{"container_restart_count", MetricClassResource},
		{"build_duration_seconds", MetricClassResource},
		{"deploy_count", MetricClassResource},
		{"brand_new_metric", MetricClassResource},
		{"", MetricClassResource},
	}
	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			if got := ClassifyMetric(tt.metric); got != tt.want {
				t.Fatalf("ClassifyMetric(%q) = %q, want %q", tt.metric, got, tt.want)
			}
		})
	}
}

func TestSilenceMatcher_MetricsNarrowing(t *testing.T) {
	m := SilenceMatcher{Apps: []string{"web"}, Kinds: []string{"threshold", "domain_health"}, Metrics: RequestPathMetrics()}
	tests := []struct {
		name string
		c    AlertContext
		want bool
	}{
		{"error rate muted", AlertContext{App: "web", Kind: "threshold", Metric: "http_responses_5xx"}, true},
		{"cpu not muted", AlertContext{App: "web", Kind: "threshold", Metric: "cpu_percent"}, false},
		{"unknown metric not muted", AlertContext{App: "web", Kind: "threshold", Metric: "mystery"}, false},
		{"domain health muted", AlertContext{App: "web", Kind: "domain_health"}, true},
		{"other app", AlertContext{App: "api", Kind: "threshold", Metric: "http_responses_5xx"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Matches(tt.c); got != tt.want {
				t.Fatalf("Matches = %v, want %v", got, tt.want)
			}
		})
	}
	if err := (SilenceMatcher{Metrics: []string{"cpu_percent"}}).Validate(); err == nil {
		t.Fatal("metrics without kinds must be rejected")
	}
}

func TestRoute_MaintenanceStyleSilenceMutesOnlyRequestPath(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	sil := Silence{ID: "sil_m", Matchers: SilenceMatcher{Apps: []string{"web"}, Kinds: []string{"threshold"}, Metrics: RequestPathMetrics()},
		StartsAt: t0.Add(-time.Minute), EndsAt: t0.Add(time.Hour), CreatedBy: "maintenance:a.example.com", CreatedAt: t0}
	if err := db.CreateSilence(ctx, sil); err != nil {
		t.Fatal(err)
	}
	cpu, errRate := webRule("cpu"), webRule("err")
	cpu.Metric, errRate.Metric = "cpu_percent", "http_responses_5xx"

	nc.Route(ctx, Event{Rule: cpu}, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatalf("cpu threshold must still notify, sent %d", len(spy.events))
	}
	nc.Route(ctx, Event{Rule: errRate}, t0, spy.send)
	if len(spy.events) != 1 {
		t.Fatalf("error rate threshold must be muted, sent %d", len(spy.events))
	}
}
