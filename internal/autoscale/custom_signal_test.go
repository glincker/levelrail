package autoscale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const gaugeText = "# TYPE queue_depth gauge\nqueue_depth 12.5\nqueue_depth_total 99\nother{a=\"b\"} 3\nq{shard=\"1\"} 4\nq{shard=\"2\"} 9\n"

func TestParseGauge(t *testing.T) {
	if v, ok := ParseGauge(strings.NewReader(gaugeText), "queue_depth"); !ok || v != 12.5 {
		t.Fatalf("queue_depth = %v %v", v, ok)
	}
	if v, ok := ParseGauge(strings.NewReader(gaugeText), "q"); !ok || v != 9 {
		t.Fatalf("labeled max = %v %v", v, ok)
	}
	if _, ok := ParseGauge(strings.NewReader(gaugeText), "missing"); ok {
		t.Fatal("missing gauge found")
	}
}

func TestScrapeGauge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(gaugeText))
	}))
	defer srv.Close()
	v, err := ScrapeGauge(context.Background(), srv.Client(), srv.URL, "/metrics", "queue_depth", DefaultGaugeMaxBody)
	if err != nil || v != 12.5 {
		t.Fatalf("v=%v err=%v", v, err)
	}
	if _, err := ScrapeGauge(context.Background(), srv.Client(), srv.URL, "/nope", "queue_depth", DefaultGaugeMaxBody); err == nil {
		t.Fatal("expected status error")
	}
}

func TestGaugePolicyValidates(t *testing.T) {
	p := Policy{App: "web", MinReplicas: 1, MaxReplicas: 5, Metric: "gauge:queue_depth", Target: 10}.Normalize()
	if err := p.Validate(20); err != nil {
		t.Fatal(err)
	}
	p.Metric = "gauge:bad name"
	if err := p.Validate(20); err == nil {
		t.Fatal("bad gauge name accepted")
	}
}

func TestGaugeScalesOutAndRespectsCooldown(t *testing.T) {
	p := Policy{App: "web", MinReplicas: 1, MaxReplicas: 10, Metric: "gauge:queue_depth", Target: 10}.Normalize()
	d := Decide(p, Input{Now: t0, Current: 2, Value: 30, ValueKnown: true})
	if d.Desired <= 2 || d.Reason != ReasonScaledUp {
		t.Fatalf("want scale out, got %+v", d)
	}
	d = Decide(p, Input{Now: t0, Current: 2, Value: 30, ValueKnown: true, LastScaleUp: t0.Add(-10 * time.Second)})
	if d.Desired != 2 || d.Reason != ReasonCooldown {
		t.Fatalf("want cooldown hold, got %+v", d)
	}
}
