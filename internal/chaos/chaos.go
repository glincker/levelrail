// Package chaos describes the failure scenarios the platform is measured against and the result record they produce.
package chaos

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Scenario names, stable because they are stored and used as CLI arguments.
const (
	KillContainer      = "kill-container"
	DaemonUnreachable  = "daemon-unreachable"
	ControlPlaneBounce = "control-plane-restart"
	DiskPressure       = "disk-pressure"
	OOMDuringReadiness = "oom-during-readiness"
	IngressSteady      = "ingress-steady"
	DeployLatency      = "deploy-latency"
	RolloutUnderLoad   = "rollout-under-traffic"
	IngressConcurrency = "ingress-concurrency"
	ReplicaFailover    = "replica-failover"
)

// Result statuses.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Info is one scenario's catalog entry.
type Info struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Expectation string `json:"expectation"`
}

// Catalog lists every scenario in display order.
func Catalog() []Info {
	return []Info{
		{KillContainer, "Kill the serving container under traffic", "a replacement becomes healthy and the ingress routes to it again"},
		{DaemonUnreachable, "Docker API unreachable during a deploy", "the deploy resumes or fails cleanly and the serving container keeps answering"},
		{ControlPlaneBounce, "Control plane restart under traffic", "the app container is untouched and the ingress comes back by itself"},
		{DiskPressure, "Scratch volume filled", "the node disk space rule fires on the measured usage"},
		{OOMDuringReadiness, "OOM kill during the readiness wait", "the deploy fails as OOMKilledDuringReadiness and the old version keeps serving"},
		{IngressSteady, "Rising steady load through the ingress", "the lowest step runs clean; the highest clean step is recorded as this machine's ceiling"},
		{DeployLatency, "Deploy latency from image to serving", "each redeploy becomes healthy and routed; p50 and p95 over the runs are recorded"},
		{RolloutUnderLoad, "Rolling deploy of two replicas under traffic", "no request fails while the new version replaces the old one"},
		{IngressConcurrency, "Ingress throughput at fixed concurrency", "requests per second and p50, p95, p99 are recorded at each concurrency with no errors"},
		{ReplicaFailover, "Kill one of two replicas under traffic", "the surviving replica keeps answering and the second replica is replaced"},
	}
}

// Valid reports whether name is a known scenario.
func Valid(name string) bool {
	for _, i := range Catalog() {
		if i.Name == name {
			return true
		}
	}
	return false
}

// Assertion is one checked expectation.
type Assertion struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Result is one measured run of one scenario.
type Result struct {
	ID            string      `json:"id,omitempty"`
	Scenario      string      `json:"scenario"`
	Status        string      `json:"status"`
	Summary       string      `json:"summary"`
	RecoveryMS    float64     `json:"recovery_ms"`
	ErrorWindowMS float64     `json:"error_window_ms"`
	Requests      int         `json:"requests"`
	RequestErrors int         `json:"request_errors"`
	Assertions    []Assertion `json:"assertions"`
	Metrics       Metrics     `json:"metrics,omitempty"`
	Hardware      string      `json:"hardware,omitempty"`
	RanAt         time.Time   `json:"ran_at"`
	DurationMS    float64     `json:"duration_ms"`
}

// Metrics holds named numeric measurements, kept in result files but not in the control plane store.
type Metrics map[string]float64

// Set records one measurement on r.
func (r *Result) Set(name string, v float64) {
	if r.Metrics == nil {
		r.Metrics = Metrics{}
	}
	r.Metrics[name] = v
}

// Assert records one expectation on r and downgrades the status when it fails.
func (r *Result) Assert(name string, ok bool, detail string) {
	r.Assertions = append(r.Assertions, Assertion{Name: name, Passed: ok, Detail: detail})
}

// Finish sets the status from the recorded assertions unless the run was skipped.
func (r *Result) Finish() {
	if r.Status == StatusSkipped {
		return
	}
	r.Status = StatusPassed
	for _, a := range r.Assertions {
		if !a.Passed {
			r.Status = StatusFailed
		}
	}
}

// Summary is the machine-readable output of one runner invocation.
type Summary struct {
	Hardware string   `json:"hardware"`
	Results  []Result `json:"results"`
	Passed   int      `json:"passed"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
}

// Summarize tallies results.
func Summarize(hardware string, results []Result) Summary {
	s := Summary{Hardware: hardware, Results: results}
	for _, r := range results {
		switch r.Status {
		case StatusPassed:
			s.Passed++
		case StatusSkipped:
			s.Skipped++
		default:
			s.Failed++
		}
	}
	return s
}

// WriteTable prints results for a human.
func WriteTable(w io.Writer, results []Result) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SCENARIO\tSTATUS\tRECOVERY\tERR WINDOW\tREQUESTS\tERRORS\tSUMMARY")
	for _, r := range results {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n", r.Scenario, r.Status,
			fmtMS(r.RecoveryMS), fmtMS(r.ErrorWindowMS), r.Requests, r.RequestErrors, r.Summary)
	}
	_ = tw.Flush()
	for _, r := range results {
		for _, a := range r.Assertions {
			if !a.Passed {
				_, _ = fmt.Fprintf(w, "FAILED %s: %s (%s)\n", r.Scenario, a.Name, a.Detail)
			}
		}
	}
}

func fmtMS(ms float64) string {
	if ms <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f ms", ms)
}

// Latest keeps the newest result per scenario, in catalog order, and names the scenarios never run.
func Latest(results []Result) (latest []Result, unmeasured []string) {
	newest := map[string]Result{}
	for _, r := range results {
		if cur, ok := newest[r.Scenario]; !ok || r.RanAt.After(cur.RanAt) {
			newest[r.Scenario] = r
		}
	}
	for _, i := range Catalog() {
		if r, ok := newest[i.Name]; ok {
			latest = append(latest, r)
		} else {
			unmeasured = append(unmeasured, i.Name)
		}
	}
	sort.Strings(unmeasured)
	return latest, unmeasured
}

// MarkdownRow renders one result as a docs/benchmarks.md table row.
func MarkdownRow(r Result) string {
	date := r.RanAt.UTC().Format("2006-01-02")
	return fmt.Sprintf("| %s | %s | %s | %s | %d of %d | %s | %s |", r.Scenario, date, r.Status,
		fmtMS(r.RecoveryMS), r.RequestErrors, r.Requests, fmtMS(r.ErrorWindowMS), strings.ReplaceAll(r.Summary, "|", "/"))
}
