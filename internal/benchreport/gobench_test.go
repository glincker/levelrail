package benchreport

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

const sampleGoBench = `goos: darwin
goarch: arm64
pkg: example.test/ingress
cpu: Apple M4 Max
BenchmarkBuildRoutesConfig/routes=10-14         	    1000	      2000 ns/op	     800 B/op	      20 allocs/op
BenchmarkBuildRoutesConfig/routes=10-14         	    1000	      4000 ns/op	     800 B/op	      20 allocs/op
BenchmarkBuildRoutesConfig/routes=10-14         	    1000	      3000 ns/op	     800 B/op	      22 allocs/op
BenchmarkV2WatchFanout/streams=1-14             	       3	  25422569 ns/op	        26.31 ms/event-max	   55970 B/op	     666 allocs/op
BenchmarkSplit/streams=2-14                     	{"level":"info","msg":"INFO log mid benchmark"}
       3	  1000 ns/op	      10 B/op	       1 allocs/op
BenchmarkNoMem-8                              	     500	       100.5 ns/op
--- BENCH: BenchmarkSomething
    some_test.go:12: log line 5 ns/op
PASS
ok  	example.test/ingress	1.2s
`

func TestParseGoBenchMedianAndUnits(t *testing.T) {
	t.Parallel()
	got, err := ParseGoBench(strings.NewReader(sampleGoBench), time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("results = %d, want 4: %+v", len(got), got)
	}
	build := got[0]
	if build.Scenario != "BenchmarkBuildRoutesConfig/routes=10" || build.Status != chaos.StatusPassed {
		t.Fatalf("first result = %+v", build)
	}
	if build.Metrics[MetricNsPerOp] != 3000 || build.Metrics[MetricAllocsPerOp] != 20 {
		t.Fatalf("median metrics = %+v", build.Metrics)
	}
	if got[1].Metrics["ms_per_event-max"] != 26.31 {
		t.Fatalf("custom metric missing: %+v", got[1].Metrics)
	}
	if got[2].Scenario != "BenchmarkSplit/streams=2" || got[2].Metrics[MetricNsPerOp] != 1000 {
		t.Fatalf("split line = %+v", got[2])
	}
	if _, ok := got[3].Metrics[MetricAllocsPerOp]; ok || got[3].Metrics[MetricNsPerOp] != 100.5 {
		t.Fatalf("no-benchmem line = %+v", got[3].Metrics)
	}
}

func TestGoBenchHost(t *testing.T) {
	t.Parallel()
	h := GoBenchHost(sampleGoBench)
	if h.OS != "darwin" || h.Arch != "arm64" || h.CPUModel != "Apple M4 Max" {
		t.Fatalf("host = %+v", h)
	}
}

func TestCompareGoBench(t *testing.T) {
	t.Parallel()
	mk := func(name string, ns, allocs float64) chaos.Result {
		r := chaos.Result{Scenario: name}
		r.Set(MetricNsPerOp, ns)
		r.Set(MetricAllocsPerOp, allocs)
		return r
	}
	base := []chaos.Result{mk("A", 100, 10), mk("B", 100, 10), mk("Skip", 100, 10)}
	head := []chaos.Result{mk("A", 130, 10), mk("B", 100, 13), mk("Skip", 900, 90), mk("New", 1, 1)}
	skip := func(n string) bool { return n == "Skip" }

	got := CompareGoBench(base, head, 25, true, skip)
	if len(got) != 2 || got[0].Name != "A" || got[0].Metric != MetricNsPerOp || got[1].Name != "B" || got[1].Metric != MetricAllocsPerOp {
		t.Fatalf("regressions = %+v", got)
	}
	if got := CompareGoBench(base, head, 25, false, skip); len(got) != 1 || got[0].Name != "B" {
		t.Fatalf("without time metrics = %+v", got)
	}
	if got := CompareGoBench(base, head, 50, true, skip); len(got) != 0 {
		t.Fatalf("within tolerance = %+v", got)
	}
}

// TestGoBenchGuard is the entry scripts/bench-guard.sh drives; it is skipped unless a benchmark output file is given.
func TestGoBenchGuard(t *testing.T) {
	outPath := os.Getenv("LEVELRAIL_BENCH_OUTPUT")
	basePath := os.Getenv("LEVELRAIL_BENCH_BASELINE")
	if outPath == "" || basePath == "" {
		t.Skip("LEVELRAIL_BENCH_OUTPUT and LEVELRAIL_BENCH_BASELINE not set")
	}
	raw, err := os.ReadFile(outPath) //nolint:gosec // path comes from the guard script
	if err != nil {
		t.Fatal(err)
	}
	head, err := ParseGoBench(strings.NewReader(string(raw)), time.Now())
	if err != nil || len(head) == 0 {
		t.Fatalf("parse %s: %d results, err = %v", outPath, len(head), err)
	}
	host := GoBenchHost(string(raw))
	if os.Getenv("LEVELRAIL_BENCH_UPDATE") == "1" {
		rep := New(head, Options{Host: host, Repeat: 1, Now: time.Now()})
		if err := WriteJSON(basePath, rep); err != nil {
			t.Fatal(err)
		}
		t.Logf("baseline written to %s (%d benchmarks)", basePath, len(head))
		return
	}
	baseRaw, err := os.ReadFile(basePath) //nolint:gosec // path comes from the guard script
	if err != nil {
		t.Fatalf("read baseline: %v (create it with LEVELRAIL_BENCH_UPDATE=1)", err)
	}
	var base Report
	if err := json.Unmarshal(baseRaw, &base); err != nil {
		t.Fatalf("decode baseline: %v", err)
	}
	tol, err := strconv.ParseFloat(os.Getenv("LEVELRAIL_BENCH_TOLERANCE_PCT"), 64)
	if err != nil || tol <= 0 {
		tol = 25
	}
	info := os.Getenv("LEVELRAIL_BENCH_INFO_PATTERN")
	if info == "" {
		info = "V2WatchFanout"
	}
	skip := func(name string) bool {
		for _, p := range strings.Split(info, ",") {
			if p = strings.TrimSpace(p); p != "" && strings.Contains(name, p) {
				return true
			}
		}
		return false
	}
	// Wall time only transfers between runs on the same CPU model; allocations transfer everywhere.
	sameHost := base.Host.CPUModel != "" && base.Host.CPUModel == host.CPUModel
	if !sameHost {
		t.Logf("baseline cpu %q differs from %q: comparing allocations only", base.Host.CPUModel, host.CPUModel)
	}
	regs := CompareGoBench(base.Results, head, tol, sameHost, skip)
	for _, r := range regs {
		t.Errorf("regression: %s %s %.0f -> %.0f (+%.1f%%, tolerance %.0f%%)", r.Name, r.Metric, r.Base, r.Head, r.Pct, tol)
	}
}
