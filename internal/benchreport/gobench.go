package benchreport

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

// Metric names a Go benchmark result is stored under in chaos.Result.Metrics.
const (
	MetricNsPerOp     = "ns_op"
	MetricBytesPerOp  = "b_op"
	MetricAllocsPerOp = "allocs_op"
)

// goBenchUnits maps the units `go test -bench -benchmem` prints to metric names.
var goBenchUnits = map[string]string{
	"ns/op":     MetricNsPerOp,
	"B/op":      MetricBytesPerOp,
	"allocs/op": MetricAllocsPerOp,
}

// ParseGoBench reads `go test -bench -benchmem` output and returns one result per benchmark,
// taking the median across repeated -count runs so one noisy run does not move the number.
func ParseGoBench(r io.Reader, ranAt time.Time) ([]chaos.Result, error) {
	samples := map[string]map[string][]float64{}
	var order []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	pending := ""
	for sc.Scan() {
		line := sc.Text()
		// A log line written mid-benchmark splits the name from its numbers.
		if f := strings.Fields(line); len(f) > 0 && strings.HasPrefix(f[0], "Benchmark") && (len(f) == 1 || strings.HasPrefix(f[1], "{")) {
			pending = f[0]
			continue
		}
		if pending != "" {
			if f := strings.Fields(line); len(f) > 0 && !strings.HasPrefix(f[0], "{") {
				if _, err := strconv.ParseInt(f[0], 10, 64); err == nil {
					line = pending + " " + line
				}
				pending = ""
			}
		}
		name, metrics, ok := parseGoBenchLine(line)
		if !ok {
			continue
		}
		if samples[name] == nil {
			samples[name] = map[string][]float64{}
			order = append(order, name)
		}
		for k, v := range metrics {
			samples[name][k] = append(samples[name][k], v)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("benchreport: read go bench output: %w", err)
	}
	results := make([]chaos.Result, 0, len(order))
	for _, name := range order {
		res := chaos.Result{ID: name, Scenario: name, Summary: "go benchmark", RanAt: ranAt.UTC()}
		for k, vs := range samples[name] {
			res.Set(k, median(vs))
		}
		res.Status = chaos.StatusPassed
		results = append(results, res)
	}
	return results, nil
}

// parseGoBenchLine decodes "BenchmarkX/sub-14  100  12345 ns/op  64 B/op  2 allocs/op [custom unit ...]".
func parseGoBenchLine(line string) (string, map[string]float64, bool) {
	f := strings.Fields(line)
	if len(f) < 4 || !strings.HasPrefix(f[0], "Benchmark") {
		return "", nil, false
	}
	if _, err := strconv.ParseInt(f[1], 10, 64); err != nil {
		return "", nil, false
	}
	metrics := map[string]float64{}
	for i := 2; i+1 < len(f); i += 2 {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return "", nil, false
		}
		unit := f[i+1]
		if name, known := goBenchUnits[unit]; known {
			metrics[name] = v
			continue
		}
		metrics[strings.ReplaceAll(unit, "/", "_per_")] = v
	}
	if _, ok := metrics[MetricNsPerOp]; !ok {
		return "", nil, false
	}
	return trimProcSuffix(f[0]), metrics, true
}

// trimProcSuffix drops the "-14" GOMAXPROCS suffix so results compare across machines.
func trimProcSuffix(name string) string {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return name
	}
	if _, err := strconv.Atoi(name[i+1:]); err != nil {
		return name
	}
	return name[:i]
}

func median(vs []float64) float64 {
	s := slices.Clone(vs)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// GoBenchHost reads the goos, goarch and cpu header lines `go test -bench` prints.
func GoBenchHost(output string) Host {
	var h Host
	for _, line := range strings.Split(output, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			continue
		}
		switch k {
		case "goos":
			h.OS = v
		case "goarch":
			h.Arch = v
		case "cpu":
			h.CPUModel = v
		}
	}
	return h
}

// GoBenchRegression is one benchmark metric that grew past the tolerance.
type GoBenchRegression struct {
	Name   string
	Metric string
	Base   float64
	Head   float64
	Pct    float64
}

// CompareGoBench lists metrics in head that exceed base by more than tolerancePct. Benchmarks
// missing from either side are ignored, and timeMetrics controls whether ns/op is compared,
// since wall time only transfers between runs on the same kind of host.
func CompareGoBench(base, head []chaos.Result, tolerancePct float64, timeMetrics bool, skip func(name string) bool) []GoBenchRegression {
	byName := make(map[string]chaos.Result, len(base))
	for _, r := range base {
		byName[r.Scenario] = r
	}
	metrics := []string{MetricAllocsPerOp, MetricBytesPerOp}
	if timeMetrics {
		metrics = append(metrics, MetricNsPerOp)
	}
	var out []GoBenchRegression
	for _, h := range head {
		b, ok := byName[h.Scenario]
		if !ok || (skip != nil && skip(h.Scenario)) {
			continue
		}
		for _, m := range metrics {
			bv, bok := b.Metrics[m]
			hv, hok := h.Metrics[m]
			if !bok || !hok || bv <= 0 {
				continue
			}
			if pct := (hv - bv) / bv * 100; pct > tolerancePct {
				out = append(out, GoBenchRegression{Name: h.Scenario, Metric: m, Base: bv, Head: hv, Pct: pct})
			}
		}
	}
	return out
}
