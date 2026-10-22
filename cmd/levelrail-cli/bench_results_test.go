package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/GLINCKER/levelrail/internal/chaos"
)

func benchResultsServer(t *testing.T) (*httptest.Server, func(string) (string, bool)) {
	t.Helper()
	res := chaos.Result{Scenario: chaos.IngressConcurrency, Status: chaos.StatusPassed}
	res.Set("achieved_rps", 900)
	rep := benchreport.New([]chaos.Result{res}, benchreport.Options{
		Build: benchreport.Build{Version: "v1", Commit: "abcdef0123456789"}, Host: benchreport.Host{OS: "darwin", Arch: "arm64", CPUs: 8}, Repeat: 1, Now: time.Unix(1700000000, 0),
	})
	var uploaded []byte
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/benchmarks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.BenchmarkList{Keep: 5, Runs: []apiclient.BenchmarkRunSummary{{ID: "bench_1", Commit: rep.Build.Commit, Version: "v1", HostSummary: "darwin/arm64", Passed: 1, Repeat: 1}}})
	})
	mux.HandleFunc("GET /api/v1/benchmarks/compare", func(w http.ResponseWriter, r *http.Request) {
		c := benchreport.Compare(rep, rep, benchreport.Side{ID: r.URL.Query().Get("base")}, benchreport.Side{ID: r.URL.Query().Get("head")}, 5)
		_ = json.NewEncoder(w).Encode(c)
	})
	mux.HandleFunc("GET /api/v1/benchmarks/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.BenchmarkRun{Run: apiclient.BenchmarkRunSummary{ID: "bench_1"}, Report: rep})
	})
	mux.HandleFunc("POST /api/v1/benchmarks", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		uploaded = buf.Bytes()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.BenchmarkRunSummary{ID: "bench_up"})
	})
	mux.HandleFunc("DELETE /api/v1/benchmarks/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		if uploaded != nil && !json.Valid(uploaded) {
			t.Error("uploaded body is not JSON")
		}
	})
	return srv, func(k string) (string, bool) {
		switch k {
		case envAPIURL:
			return srv.URL, true
		case envAPIToken:
			return "tok", true
		}
		return "", false
	}
}

func TestBenchResultsSubcommands(t *testing.T) {
	_, env := benchResultsServer(t)
	file := filepath.Join(t.TempDir(), "r.json")
	if err := os.WriteFile(file, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"list"}, "bench_1"},
		{[]string{"show", "bench_1"}, "NOT Linux production numbers"},
		{[]string{"compare", "bench_1", "bench_2"}, "achieved_rps"},
		{[]string{"upload", file}, "stored as bench_up"},
		{[]string{"delete", "bench_1"}, "deleted bench_1"},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if got := runBench("levelrail-cli", append([]string{"results"}, c.args...), &stdout, &stderr, env); got != exitOK {
			t.Errorf("%v: exit %d stderr=%q", c.args, got, stderr.String())
			continue
		}
		if !strings.Contains(stdout.String(), c.want) {
			t.Errorf("%v: stdout lacks %q:\n%s", c.args, c.want, stdout.String())
		}
	}
}

func TestBenchResultsArgumentCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := runBench("levelrail-cli", []string{"results", "compare", "only-one"}, &stdout, &stderr, envMap()); got != exitValidation {
		t.Errorf("exit = %d, want %d", got, exitValidation)
	}
	if got := runBench("levelrail-cli", []string{"results", "nope"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Errorf("unknown sub exit = %d, want %d", got, exitUsage)
	}
}
