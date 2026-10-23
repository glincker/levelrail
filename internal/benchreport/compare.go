package benchreport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

// Scenario groups the dashboard and the compare view use.
const (
	GroupFailover   = "failover"
	GroupDeploy     = "deploy"
	GroupRollout    = "rollout"
	GroupThroughput = "throughput"
)

// Directions of a delta, judged by what a larger number means for the metric.
const (
	DirectionBetter = "better"
	DirectionWorse  = "worse"
	DirectionSame   = "same"
)

// Group names the section a scenario's numbers belong to.
func Group(scenario string) string {
	switch scenario {
	case chaos.DeployLatency:
		return GroupDeploy
	case chaos.RolloutUnderLoad:
		return GroupRollout
	case chaos.IngressSteady, chaos.IngressConcurrency:
		return GroupThroughput
	default:
		return GroupFailover
	}
}

// Parse decodes and validates an uploaded results file. Unknown fields are rejected so a mistyped file is not stored half understood.
func Parse(data []byte) (Report, error) {
	var r Report
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Report{}, fmt.Errorf("benchreport: decode results file: %w", err)
	}
	if r.SchemaVersion != SchemaVersion {
		return Report{}, fmt.Errorf("benchreport: schema version %d is not supported, this server reads version %d", r.SchemaVersion, SchemaVersion)
	}
	if err := r.Validate(); err != nil {
		return Report{}, err
	}
	for _, res := range r.Results {
		if !chaos.Valid(res.Scenario) {
			return Report{}, fmt.Errorf("benchreport: unknown scenario %q", res.Scenario)
		}
		switch res.Status {
		case chaos.StatusPassed, chaos.StatusFailed, chaos.StatusSkipped:
		default:
			return Report{}, fmt.Errorf("benchreport: scenario %s has status %q", res.Scenario, res.Status)
		}
	}
	return r, nil
}

// Tally counts results by status.
func (r Report) Tally() (passed, failed, skipped int) {
	for _, res := range r.Results {
		switch res.Status {
		case chaos.StatusPassed:
			passed++
		case chaos.StatusSkipped:
			skipped++
		default:
			failed++
		}
	}
	return passed, failed, skipped
}

// Side identifies one report in a comparison.
type Side struct {
	ID          string `json:"id"`
	GeneratedAt string `json:"generated_at"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Dirty       bool   `json:"dirty"`
	Host        string `json:"host"`
	Runs        int    `json:"runs"`
}

// Delta is one metric on both sides. Each side is the median over that side's runs of the scenario.
type Delta struct {
	Scenario  string   `json:"scenario"`
	Group     string   `json:"group"`
	Metric    string   `json:"metric"`
	Base      *float64 `json:"base"`
	Head      *float64 `json:"head"`
	Delta     *float64 `json:"delta"`
	DeltaPct  *float64 `json:"delta_pct"`
	Direction string   `json:"direction,omitempty"`
}

// StatusChange lists a scenario whose outcome differs between the sides.
type StatusChange struct {
	Scenario string `json:"scenario"`
	Base     string `json:"base"`
	Head     string `json:"head"`
}

// Comparison is the head report measured against the base report.
type Comparison struct {
	Base      Side           `json:"base"`
	Head      Side           `json:"head"`
	Deltas    []Delta        `json:"deltas"`
	Status    []StatusChange `json:"status_changes"`
	Warnings  []string       `json:"warnings"`
	NoisePct  float64        `json:"noise_pct"`
	Statistic string         `json:"statistic"`
}

// Compare lines up every metric of two reports. A change smaller than noisePct percent is reported as the same.
func Compare(base, head Report, baseSide, headSide Side, noisePct float64) Comparison {
	baseSide.Runs, headSide.Runs = base.Repeat, head.Repeat
	c := Comparison{Base: baseSide, Head: headSide, Deltas: []Delta{}, Status: []StatusChange{}, NoisePct: noisePct, Statistic: "median over runs"}
	bm, hm := medians(base.Results), medians(head.Results)
	keys := map[[2]string]bool{}
	for k := range bm {
		keys[k] = true
	}
	for k := range hm {
		keys[k] = true
	}
	order := scenarioIndex()
	sorted := make([][2]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a[0] != b[0] {
			return order[a[0]] < order[b[0]]
		}
		return a[1] < b[1]
	})
	for _, k := range sorted {
		d := Delta{Scenario: k[0], Group: Group(k[0]), Metric: k[1]}
		bv, bok := bm[k]
		hv, hok := hm[k]
		if bok {
			d.Base = &bv
		}
		if hok {
			d.Head = &hv
		}
		if bok && hok {
			diff := hv - bv
			d.Delta = &diff
			if bv != 0 {
				pct := diff / math.Abs(bv) * 100
				d.DeltaPct = &pct
			}
			d.Direction = direction(k[1], bv, hv, noisePct)
		}
		c.Deltas = append(c.Deltas, d)
	}
	c.Status = statusChanges(base.Results, head.Results)
	c.Warnings = compareWarnings(base, head)
	return c
}

func scenarioIndex() map[string]int {
	m := map[string]int{}
	for i, info := range chaos.Catalog() {
		m[info.Name] = i
	}
	return m
}

func higherIsBetter(metric string) bool {
	return strings.HasSuffix(metric, "rps") || strings.HasPrefix(metric, "highest_clean")
}

func direction(metric string, base, head, noisePct float64) string {
	if base == head {
		return DirectionSame
	}
	if base != 0 && math.Abs((head-base)/base*100) < noisePct {
		return DirectionSame
	}
	up := head > base
	if up == higherIsBetter(metric) {
		return DirectionBetter
	}
	return DirectionWorse
}

func medians(results []chaos.Result) map[[2]string]float64 {
	samples := map[[2]string][]float64{}
	add := func(res chaos.Result, name string, v float64) {
		k := [2]string{res.Scenario, name}
		samples[k] = append(samples[k], v)
	}
	for _, res := range results {
		if res.Status == chaos.StatusSkipped {
			continue
		}
		add(res, "recovery_ms", res.RecoveryMS)
		add(res, "error_window_ms", res.ErrorWindowMS)
		add(res, "request_errors", float64(res.RequestErrors))
		for name, v := range res.Metrics {
			add(res, name, v)
		}
	}
	out := make(map[[2]string]float64, len(samples))
	for k, vs := range samples {
		sort.Float64s(vs)
		mid := len(vs) / 2
		if len(vs)%2 == 1 {
			out[k] = vs[mid]
		} else {
			out[k] = (vs[mid-1] + vs[mid]) / 2
		}
	}
	return out
}

func statusChanges(base, head []chaos.Result) []StatusChange {
	b, h := worstStatus(base), worstStatus(head)
	var out []StatusChange
	for _, info := range chaos.Catalog() {
		bs, hs := b[info.Name], h[info.Name]
		if bs == "" {
			bs = "not run"
		}
		if hs == "" {
			hs = "not run"
		}
		if bs != hs {
			out = append(out, StatusChange{Scenario: info.Name, Base: bs, Head: hs})
		}
	}
	if out == nil {
		out = []StatusChange{}
	}
	return out
}

func worstStatus(results []chaos.Result) map[string]string {
	rank := map[string]int{chaos.StatusSkipped: 1, chaos.StatusPassed: 2, chaos.StatusFailed: 3}
	out := map[string]string{}
	for _, r := range results {
		if rank[r.Status] > rank[out[r.Scenario]] {
			out[r.Scenario] = r.Status
		}
	}
	return out
}

func compareWarnings(base, head Report) []string {
	w := []string{}
	if base.Host.OS != head.Host.OS || base.Host.Arch != head.Host.Arch {
		w = append(w, fmt.Sprintf("The runs are from different platforms (%s/%s and %s/%s), so the deltas compare machines, not builds.", base.Host.OS, base.Host.Arch, head.Host.OS, head.Host.Arch))
	} else if base.Host.CPUs != head.Host.CPUs || base.Host.CPUModel != head.Host.CPUModel || base.Host.MemBytes != head.Host.MemBytes {
		w = append(w, "The runs are from differently sized hosts, so the deltas compare machines as well as builds.")
	}
	if base.Host.Docker.Version != head.Host.Docker.Version {
		w = append(w, "The Docker engine version differs between the runs.")
	}
	if base.Build.Dirty || head.Build.Dirty {
		w = append(w, "At least one run was built from a tree with uncommitted changes.")
	}
	if base.Repeat < 2 || head.Repeat < 2 {
		w = append(w, "At least one side is a single run per scenario, so a delta can be noise.")
	}
	if base.Host.OS != "linux" || head.Host.OS != "linux" {
		w = append(w, "NOT Linux production numbers: at least one run was taken on a non-Linux host.")
	}
	return w
}
