package benchreport

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

// SchemaVersion changes when a field of the results file changes meaning.
const SchemaVersion = 1

// Build identifies the binary that produced the numbers.
type Build struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Dirty   bool   `json:"dirty"`
}

// Limit states whether one question is answered by the results.
type Limit struct {
	Question string `json:"question"`
	Status   string `json:"status"`
}

// Report is the results file: numbers plus everything needed to judge and repeat them.
type Report struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Build         Build          `json:"build"`
	Host          Host           `json:"host"`
	Repeat        int            `json:"repeat"`
	Methodology   []string       `json:"methodology"`
	Caveats       []string       `json:"caveats"`
	Results       []chaos.Result `json:"results"`
	Limits        []Limit        `json:"limits"`
}

// Options carries the inputs New cannot read from the results themselves.
type Options struct {
	Build          Build
	Host           Host
	Repeat         int
	Now            time.Time
	NoisyLoadRatio float64
}

// New assembles a report, deriving the caveats and the limits table from what was actually run.
func New(results []chaos.Result, o Options) Report {
	r := Report{
		SchemaVersion: SchemaVersion, GeneratedAt: o.Now.UTC(), Build: o.Build, Host: o.Host,
		Repeat: max(o.Repeat, 1), Methodology: Methodology(), Results: results,
	}
	r.Caveats = Caveats(o.Host, o.Build, r.Repeat, o.NoisyLoadRatio)
	r.Limits = Limits(results, r.Repeat)
	return r
}

// Validate rejects a report that would publish numbers without the facts needed to judge them.
func (r Report) Validate() error {
	var missing []string
	if len(r.Results) == 0 {
		missing = append(missing, "results")
	}
	if r.Build.Version == "" {
		missing = append(missing, "build version")
	}
	if r.Build.Commit == "" {
		missing = append(missing, "build commit")
	}
	if r.Host.OS == "" || r.Host.CPUs == 0 {
		missing = append(missing, "host")
	}
	if len(r.Methodology) == 0 {
		missing = append(missing, "methodology")
	}
	if r.GeneratedAt.IsZero() {
		missing = append(missing, "timestamp")
	}
	if len(missing) > 0 {
		return errors.New("benchreport: incomplete report, missing " + strings.Join(missing, ", "))
	}
	return nil
}

// Methodology lists how the numbers are taken, in the words published with them.
func Methodology() []string {
	return []string{
		"Each scenario builds its own control plane slice (store, reconcile engine, embedded ingress) in the benchmark process and deploys a trivial fixture container on the local Docker engine.",
		"The load generator runs on the same machine as the system under test, over loopback with TLS through the embedded ingress.",
		"Request load is open loop: request i is scheduled at i / rate and latency is measured from that intended send time, so a stall is not hidden (coordinated omission). Ingress concurrency steps are closed loop and say so.",
		"Percentiles are nearest rank over every request, failures included. A run with client_saturated above zero fails its own check.",
		"Recovery time is the event to the last failed request. Replica restore time is polled from the container list at 200 ms resolution.",
		"Deploy latency is Deploy to the first answer from the new version through the ingress, and includes the stability window the deploy gate waits for.",
		"With repeat above 1 every run is kept and listed; nothing is averaged or dropped.",
	}
}

// Caveats names the reasons these numbers must not be read as production numbers.
func Caveats(h Host, b Build, repeat int, noisyLoadRatio float64) []string {
	var c []string
	if h.OS != "linux" {
		c = append(c, fmt.Sprintf("NOT Linux production numbers: the host is %s and Docker runs inside a virtual machine, so container, network and disk paths differ from a Linux server.", h.OS))
	}
	if strings.Contains(strings.ToLower(h.Docker.OS), "desktop") {
		c = append(c, "Docker Desktop engine: the container runtime is a shared virtual machine, not a native Linux Docker.")
	}
	c = append(c, "Load generator and system under test share one machine, so they compete for CPU and the figures are a floor for throughput and a ceiling for latency.")
	c = append(c, "The control plane is an in-process slice, not the shipped binary under systemd, and there is one node: nothing here covers multi-node, zones or a real network.")
	if h.LoadStart >= 0 && h.CPUs > 0 {
		ratio := h.LoadStart / float64(h.CPUs)
		line := fmt.Sprintf("Host load average (1 minute) was %.2f at start and %.2f at end on %d CPUs.", h.LoadStart, h.LoadEnd, h.CPUs)
		if noisyLoadRatio > 0 && (ratio >= noisyLoadRatio || (h.LoadEnd >= 0 && h.LoadEnd/float64(h.CPUs) >= noisyLoadRatio)) {
			line += " The host was busy, so treat every latency as a noisy upper bound."
		}
		c = append(c, line)
	}
	if repeat < 2 {
		c = append(c, "Single run per scenario: this is a first measurement, not a distribution.")
	}
	if b.Commit == "" || b.Commit == "unknown" {
		c = append(c, "The binary carries no VCS stamp, so these numbers are not tied to a commit.")
	}
	if b.Dirty {
		c = append(c, "The binary was built from a tree with uncommitted changes.")
	}
	return c
}

// Limits is the table of what the run measured and what it did not.
func Limits(results []chaos.Result, repeat int) []Limit {
	byScenario := map[string][]chaos.Result{}
	for _, r := range results {
		byScenario[r.Scenario] = append(byScenario[r.Scenario], r)
	}
	var out []Limit
	for _, info := range chaos.Catalog() {
		rs := byScenario[info.Name]
		out = append(out, Limit{Question: info.Title, Status: scenarioStatus(rs)})
	}
	out = append(out,
		Limit{"Anything on a Linux server, a real network or a real application", "not measured"},
		Limit{"Node agent link loss, node drain, zone loss", "not measured: needs a second node"},
		Limit{"Control plane restart under systemd socket activation", "not measured: the restart scenario is in process"},
		Limit{"Docker daemon restart (engine stopped)", "not measured: the API is cut with a proxy, the daemon keeps running"},
		Limit{"Ingress throughput ceiling", "not established: the ladder stops at its top step, so the ceiling is at least the highest clean step"},
	)
	if repeat < 5 {
		out = append(out, Limit{"Statistical confidence", fmt.Sprintf("not measured: %d run(s) per scenario", repeat)})
	}
	return out
}

func scenarioStatus(rs []chaos.Result) string {
	if len(rs) == 0 {
		return "not run"
	}
	var passed, failed, skipped int
	for _, r := range rs {
		switch r.Status {
		case chaos.StatusPassed:
			passed++
		case chaos.StatusSkipped:
			skipped++
		default:
			failed++
		}
	}
	switch {
	case passed == 0 && failed == 0:
		return "not run: skipped, " + rs[0].Summary
	case failed > 0:
		return fmt.Sprintf("measured, %d of %d run(s) failed an assertion", failed, len(rs))
	}
	s := fmt.Sprintf("measured on this host, %d run(s), all checks passed", passed)
	if skipped > 0 {
		s += fmt.Sprintf("; %d more run(s) skipped", skipped)
	}
	return s
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(path string, r Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("benchreport: encode: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil { // #nosec G306 -- a results file is meant to be shared
		return fmt.Errorf("benchreport: write %s: %w", path, err)
	}
	return nil
}

// Markdown renders the report as a document that carries its own host, build and limits.
func Markdown(r Report) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# Benchmark results\n\n")
	w("| Field | Value |\n| --- | --- |\n")
	w("| Generated | %s |\n", r.GeneratedAt.Format("2006-01-02 15:04 MST"))
	commit := r.Build.Commit
	if r.Build.Dirty {
		commit += " (dirty tree)"
	}
	w("| Version | %s |\n| Commit | %s |\n", r.Build.Version, commit)
	w("| Host | %s |\n", cell(r.Host.Summary()))
	if r.Host.Kernel != "" {
		w("| Kernel | %s |\n", cell(r.Host.Kernel))
	}
	if r.Host.Docker.MemBytes > 0 {
		w("| Memory visible to Docker | %d GiB |\n", r.Host.Docker.MemBytes>>30)
	}
	w("| Load average at start and end | %s and %s |\n", load(r.Host.LoadStart), load(r.Host.LoadEnd))
	w("| Runs per scenario | %d |\n", r.Repeat)
	if r.Host.Note != "" {
		w("| Note | %s |\n", cell(r.Host.Note))
	}

	w("\n## Read this first\n\n")
	for _, c := range r.Caveats {
		w("- %s\n", c)
	}

	w("\n## Results\n\n| Scenario | Run | Status | Recovery | Failed requests | Error window | Summary |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	runs := runIndex(r.Results)
	for i, res := range r.Results {
		w("| %s | %d | %s | %s | %d of %d | %s | %s |\n", res.Scenario, runs[i], res.Status,
			ms(res.RecoveryMS), res.RequestErrors, res.Requests, ms(res.ErrorWindowMS), cell(res.Summary))
	}

	w("\n## Measurements\n\nValues in milliseconds unless the name ends in rps, errors, runs or saturated.\n")
	for _, name := range scenarioOrder(r.Results) {
		var group []chaos.Result
		for _, res := range r.Results {
			if res.Scenario == name && len(res.Metrics) > 0 {
				group = append(group, res)
			}
		}
		if len(group) == 0 {
			continue
		}
		w("\n### %s\n\n| Metric |", name)
		for i := range group {
			w(" Run %d |", i+1)
		}
		w("\n| --- |")
		for range group {
			w(" --- |")
		}
		w("\n")
		for _, key := range metricKeys(group) {
			w("| %s |", key)
			for _, g := range group {
				if v, ok := g.Metrics[key]; ok {
					w(" %s |", num(v))
				} else {
					w(" n/a |")
				}
			}
			w("\n")
		}
	}

	w("\n## Limits table\n\n| Question | Status |\n| --- | --- |\n")
	for _, l := range r.Limits {
		w("| %s | %s |\n", cell(l.Question), cell(l.Status))
	}

	w("\n## Method\n\n")
	for _, m := range r.Methodology {
		w("- %s\n", m)
	}
	return b.String()
}

// WriteMarkdown writes Markdown(r) to path.
func WriteMarkdown(path string, r Report) error {
	if err := os.WriteFile(path, []byte(Markdown(r)), 0o644); err != nil { // #nosec G306 -- a results file is meant to be shared
		return fmt.Errorf("benchreport: write %s: %w", path, err)
	}
	return nil
}

func runIndex(results []chaos.Result) []int {
	seen := map[string]int{}
	out := make([]int, len(results))
	for i, r := range results {
		seen[r.Scenario]++
		out[i] = seen[r.Scenario]
	}
	return out
}

func scenarioOrder(results []chaos.Result) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range results {
		if !seen[r.Scenario] {
			seen[r.Scenario] = true
			out = append(out, r.Scenario)
		}
	}
	return out
}

func metricKeys(group []chaos.Result) []string {
	set := map[string]bool{}
	for _, g := range group {
		for k := range g.Metrics {
			set[k] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func num(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}

func ms(v float64) string {
	if v <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f ms", v)
}

func load(v float64) string {
	if v < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", v)
}

func cell(s string) string { return strings.ReplaceAll(s, "|", "/") }
