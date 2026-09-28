package main

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func requestsFixture() apiclient.AppRequestsResource {
	return apiclient.AppRequestsResource{
		App:     "web",
		Summary: apiclient.RequestSummaryResource{HasTraffic: true, Requests: 100, RatePerSec: 2.5, ErrorRate5xx: 0.05, P95Ms: 180},
		Points:  []apiclient.RequestPointResource{{Timestamp: time.Unix(1_700_000_000, 0).UTC(), RatePerSec: 2.5, P95Ms: 180}},
	}
}

func TestRun_AppsRequests(t *testing.T) {
	var path string
	srv := newListEchoServer(t, &path, requestsFixture())
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "requests", "web", "--api-url", srv.URL})
	if path != "/api/v1/apps/web/requests" || !strings.Contains(stdout, "p95: 180ms") || !strings.Contains(stdout, "5xx: 5.00%") {
		t.Errorf("path %q stdout %q", path, stdout)
	}
}

func TestRun_AppsMetrics_RequestsFlag(t *testing.T) {
	var path string
	srv := newListEchoServer(t, &path, requestsFixture())
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "metrics", "web", "--requests", "--api-url", srv.URL})
	if path != "/api/v1/apps/web/requests" {
		t.Errorf("path = %q", path)
	}
}

func TestRun_AppsRequests_NoTraffic(t *testing.T) {
	srv := newListEchoServer(t, nil, apiclient.AppRequestsResource{App: "web"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "requests", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no requests in range") {
		t.Errorf("stdout = %q", stdout)
	}
}
