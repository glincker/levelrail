package main

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func trafficFixture() apiclient.AppTrafficResource {
	return apiclient.AppTrafficResource{
		App: "web", Window: "1h", HasTraffic: true, Requests: 1000, RatePerSec: 0.28,
		StatusClasses: apiclient.TrafficStatusClasses{Status2xx: 950, Status5xx: 50},
		ErrorRate5xx:  0.05,
		Latency:       apiclient.TrafficLatency{P50Ms: 20, P95Ms: 180, P99Ms: 900},
		Replicas: []apiclient.TrafficReplica{
			{Upstream: "172.18.0.2:3000", Requests: 600, TrafficLatency: apiclient.TrafficLatency{P99Ms: 800}},
		},
		SlowRoutes: []apiclient.TrafficRoute{{Route: "/api/:id", Requests: 40, AvgMs: 700}},
		Samples:    []apiclient.TrafficSample{{Time: time.Unix(1_700_000_000, 0).UTC(), Route: "/api/:id", Method: "GET", Status: 502, DurationMS: 3000, RequestID: "req1"}},
		Referrers:  []apiclient.TrafficLabel{{Label: "news.example", Requests: 30, Share: 0.3}},
		Countries:  []apiclient.TrafficLabel{{Label: "DE", Requests: 20, Share: 0.2}},
		SLO:        &apiclient.TrafficSLO{Objective: "availability", Target: 99.9, BurnRate: 4.2, BudgetRemaining: 0.4},
		ErrorSpikes: []apiclient.TrafficErrorSpike{{
			Start: time.Unix(1_700_000_000, 0).UTC(), End: time.Unix(1_700_000_060, 0).UTC(),
			PeakRate5xx: 0.6, Errors5xx: 60, DeployID: "dep9", DeployImage: "web:9",
		}},
	}
}

func TestRun_Traffic(t *testing.T) {
	var path string
	srv := newListEchoServer(t, &path, trafficFixture())
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"traffic", "web", "--window", "5m", "--api-url", srv.URL})
	if path != "/api/v1/apps/web/traffic" {
		t.Errorf("path = %q", path)
	}
	for _, want := range []string{"p99 900ms", "172.18.0.2:3000", "/api/:id", "burn 4.20x", "req1", "news.example", "30.0%", "DE", "5xx spikes", "dep9 web:9"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestRun_Traffic_NoTraffic(t *testing.T) {
	srv := newListEchoServer(t, nil, apiclient.AppTrafficResource{App: "web", Window: "1h"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"traffic", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no requests in this window") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_Traffic_RejectsBadWindow(t *testing.T) {
	runCLIExpectValidationError(t, []string{"traffic", "web", "--window", "2d"})
}
