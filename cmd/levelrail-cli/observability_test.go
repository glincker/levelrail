package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func obsServer(t *testing.T, body any) (*httptest.Server, *url.URL) {
	t.Helper()
	got := &url.URL{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.URL
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestRun_MetricsQuery(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	srv, got := obsServer(t, apiclient.MetricsSeriesResource{
		Metric: "cpu_percent", StepSeconds: 300, Downsampled: true,
		Points:         []apiclient.MetricSeriesPoint{{Timestamp: at, Value: 10, Max: 80}},
		PreviousPoints: []apiclient.MetricSeriesPoint{{Timestamp: at, Value: 5}},
	})
	stdout, _ := runCLIExpectOK(t, []string{"metrics", "query", "web", "--metric", "cpu_percent", "--max-points", "120", "--compare", "--api-url", srv.URL})
	q := got.Query()
	if got.Path != "/api/v1/apps/web/metrics" || q.Get("max_points") != "120" || q.Get("compare") != "previous" || q.Get("metric") != "cpu_percent" {
		t.Errorf("request = %s", got.String())
	}
	for _, want := range []string{"PREVIOUS", "80.00", "5.00", "downsampled"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

func TestRun_MetricsQuery_RequiresMetric(t *testing.T) {
	var out, errOut strings.Builder
	code := run("levelrail-cli-test", []string{"metrics", "query", "web"}, &out, &errOut, envMap())
	if code == exitOK {
		t.Errorf("expected a usage or validation failure, stderr=%q", errOut.String())
	}
}

func TestRankUsage(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rows := []apiclient.AppResourceUsageResource{
		{Name: "a", CPUPercent: f(5), MemoryUsageBytes: f(900)},
		{Name: "b", CPUPercent: f(50), MemoryUsageBytes: f(100)},
		{Name: "c", NetworkRxBytes: f(10), NetworkTxBytes: f(1000)},
	}
	cases := []struct {
		by    string
		limit int
		want  string
	}{
		{"cpu", 2, "b,a"},
		{"memory", 3, "a,b,c"},
		{"network", 1, "c"},
	}
	for _, tc := range cases {
		var names []string
		for _, r := range rankUsage(rows, tc.by, tc.limit) {
			names = append(names, r.Name)
		}
		if got := strings.Join(names, ","); got != tc.want {
			t.Errorf("by %s = %s, want %s", tc.by, got, tc.want)
		}
	}
}

func TestRun_LogsSearchSendsFilters(t *testing.T) {
	srv, got := obsServer(t, apiclient.LogSearchResource{
		Entries: []apiclient.LogSearchEntry{{Timestamp: time.Unix(1_700_000_000, 0).UTC(), Stream: "stderr", Message: "boom"}},
		Total:   1,
	})
	stdout, _ := runCLIExpectOK(t, []string{"logs", "search", "web", "--q", "timeout", "--level", "error", "--container", "abc", "--stream", "stderr",
		"--field", "status=500", "--field", "route~/pay", "--api-url", srv.URL})
	q := got.Query()
	if got.Path != "/api/v1/apps/web/logs" || q.Get("q") != "timeout" || q.Get("container") != "abc" || q.Get("stream") != "stderr" {
		t.Errorf("request = %s", got.String())
	}
	if f := q["field"]; len(f) != 2 || f[0] != "status=500" || f[1] != "route~/pay" {
		t.Errorf("fields = %v", f)
	}
	if !strings.Contains(stdout, "boom") || !strings.Contains(stdout, "showing 1 of 1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_LogsFailureAndInvestigate(t *testing.T) {
	srv, got := obsServer(t, apiclient.FailureContextResource{
		State: "crashlooping", Restarts: 4, WindowSeconds: 900, TotalLines: 1, LinesSource: "runtime",
		Lines: []apiclient.FailureContextLine{{Timestamp: time.Unix(1_700_000_000, 0).UTC(), Message: "panic: nil map"}},
	})
	stdout, _ := runCLIExpectOK(t, []string{"logs", "failure", "web", "--api-url", srv.URL})
	if got.Path != "/api/v1/apps/web/failure-context" || !strings.Contains(stdout, "panic: nil map") || !strings.Contains(stdout, "crashlooping: 4 restarts") {
		t.Errorf("path %s stdout %q", got.Path, stdout)
	}

	inv, got2 := obsServer(t, apiclient.InvestigationResource{
		App: "web", Summary: apiclient.InvestigateSummary{HasTraffic: true, Requests: 100, P95Ms: 900},
		TopRoutes: []apiclient.InvestigateRoute{{Route: "/api/orders", Requests: 80, Share: 0.8}},
		Timeline:  []apiclient.InvestigateEvent{{At: time.Unix(1_700_000_000, 0).UTC(), Kind: "deploy", Title: "Deployed img:2", LikelyCause: true}},
	})
	stdout, _ = runCLIExpectOK(t, []string{"metrics", "investigate", "web", "--at", "30m", "--window", "10m", "--api-url", inv.URL})
	if got2.Path != "/api/v1/apps/web/investigate" || got2.Query().Get("from") == "" {
		t.Errorf("request = %s", got2.String())
	}
	for _, want := range []string{"/api/orders", "Deployed img:2", "*", "p95 900ms"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}
