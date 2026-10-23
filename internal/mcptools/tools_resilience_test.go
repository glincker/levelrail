package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResilienceToolBenchmarkViews(t *testing.T) {
	res := chaos.Result{Scenario: chaos.IngressConcurrency, Status: chaos.StatusPassed}
	res.Set("achieved_rps", 900)
	rep := benchreport.New([]chaos.Result{res}, benchreport.Options{
		Build: benchreport.Build{Version: "v1", Commit: "abcdef0123456789"}, Host: benchreport.Host{OS: "darwin", Arch: "arm64", CPUs: 8}, Repeat: 1, Now: time.Unix(1700000000, 0),
	})
	head := rep
	head.Results = []chaos.Result{{Scenario: chaos.IngressConcurrency, Status: chaos.StatusPassed, Metrics: chaos.Metrics{"achieved_rps": 1200}}}
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/resilience":
			_ = json.NewEncoder(w).Encode(apiclient.ResilienceReport{})
		case "/api/v1/benchmarks":
			_ = json.NewEncoder(w).Encode(apiclient.BenchmarkList{Runs: []apiclient.BenchmarkRunSummary{{ID: "bench_1", Commit: "abcdef0123456789", Passed: 1, GeneratedAt: time.Unix(1700000000, 0)}}})
		case "/api/v1/benchmarks/compare":
			_ = json.NewEncoder(w).Encode(benchreport.Compare(rep, head, benchreport.Side{ID: "bench_1"}, benchreport.Side{ID: "bench_2"}, 5))
		default:
			_ = json.NewEncoder(w).Encode(apiclient.BenchmarkRun{Run: apiclient.BenchmarkRunSummary{ID: "bench_1"}, Report: rep})
		}
	})
	call := func(args map[string]any) []string {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_resilience_report", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		var out resilienceOutput
		decodeStructured(t, result, &out)
		return out.Results
	}

	if got := strings.Join(call(nil), "\n"); !strings.Contains(got, "benchmark bench_1") {
		t.Errorf("default view lacks the run list:\n%s", got)
	}
	if got := strings.Join(call(map[string]any{"run_id": "bench_1"}), "\n"); !strings.Contains(got, "caveat: NOT Linux production numbers") || !strings.Contains(got, "limit:") {
		t.Errorf("run view lacks caveat or limits:\n%s", got)
	}
	if got := strings.Join(call(map[string]any{"run_id": "bench_2", "compare_to": "bench_1"}), "\n"); !strings.Contains(got, "achieved_rps 900.00 to 1200.00 (+33.3%) better") {
		t.Errorf("compare view lacks the delta:\n%s", got)
	}
}
