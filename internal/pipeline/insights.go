package pipeline

import (
	"sort"
	"strings"
)

// Job outcomes the insight functions understand, matching store statuses.
const (
	sampleOK     = "succeeded"
	sampleFailed = "failed"
)

// JobSample is one job's outcome in one run. Key may carry a matrix suffix.
type JobSample struct {
	Key             string
	Status          string
	DurationSeconds int64
	Attempt         int
}

// RunSample is one finished run, as handed to Insights newest first.
type RunSample struct {
	RunID     string
	Number    int
	CommitSHA string
	Jobs      []JobSample
}

// JobInsight summarises one job over a window of runs.
type JobInsight struct {
	Job         string  `json:"job"`
	Runs        int     `json:"runs"`
	Failures    int     `json:"failures"`
	FailureRate float64 `json:"failure_rate"`
	P50Seconds  int64   `json:"p50_seconds"`
	LastSeconds int64   `json:"last_seconds"`
	Flaky       bool    `json:"flaky"`
	FlakyReason string  `json:"flaky_reason,omitempty"`
}

// JobDelta is how a job's duration moved between two runs.
type JobDelta struct {
	Job             string  `json:"job"`
	BaselineSeconds int64   `json:"baseline_seconds"`
	CurrentSeconds  int64   `json:"current_seconds"`
	DeltaSeconds    int64   `json:"delta_seconds"`
	DeltaPercent    float64 `json:"delta_percent"`
}

// Insights is the run-over-run view of a pipeline.
type Insights struct {
	Window     int          `json:"window"`
	Jobs       []JobInsight `json:"jobs"`
	DiffRunID  string       `json:"diff_run_id,omitempty"`
	DiffBaseID string       `json:"diff_base_run_id,omitempty"`
	Diff       []JobDelta   `json:"diff"`
}

func baseKey(k string) string {
	if i := strings.IndexByte(k, '['); i >= 0 {
		return k[:i]
	}
	return k
}

// jobTotals sums matrix instances so a job is one number per run.
func jobTotals(r RunSample) map[string]JobSample {
	out := map[string]JobSample{}
	for _, j := range r.Jobs {
		if j.Status != sampleOK && j.Status != sampleFailed {
			continue
		}
		k := baseKey(j.Key)
		cur, seen := out[k]
		if !seen {
			out[k] = JobSample{Key: k, Status: j.Status, DurationSeconds: j.DurationSeconds, Attempt: j.Attempt}
			continue
		}
		if j.Status == sampleFailed {
			cur.Status = sampleFailed
		}
		if j.DurationSeconds > cur.DurationSeconds {
			cur.DurationSeconds = j.DurationSeconds
		}
		if j.Attempt > cur.Attempt {
			cur.Attempt = j.Attempt
		}
		out[k] = cur
	}
	return out
}

func median(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]int64(nil), v...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

// flaky flags a job whose result changes without the code changing: both
// outcomes on one commit, success only after a retry, or frequent flips at a
// failure rate that is neither rare nor constant.
func flaky(results []string, sameCommitMixed, retriedOK bool) (bool, string) {
	if sameCommitMixed {
		return true, "passed and failed on the same commit"
	}
	if retriedOK {
		return true, "passed only after a retry"
	}
	flips := 0
	for i := 1; i < len(results); i++ {
		if results[i] != results[i-1] {
			flips++
		}
	}
	fails := 0
	for _, r := range results {
		if r == sampleFailed {
			fails++
		}
	}
	if len(results) >= 6 && flips >= 3 && fails*10 >= len(results) && fails*10 <= len(results)*9 {
		return true, "result flips between runs"
	}
	return false, ""
}

// ComputeInsights builds per-job stats and flaky flags over runs (newest
// first), plus a duration diff of the newest run against the latest earlier
// run in which every shared job succeeded.
func ComputeInsights(runs []RunSample) Insights {
	res := Insights{Window: len(runs), Jobs: []JobInsight{}, Diff: []JobDelta{}}
	type acc struct {
		durs       []int64
		results    []string
		last       int64
		byCommit   map[string]map[string]bool
		retriedOK  bool
		failures   int
		seenLatest bool
	}
	accs := map[string]*acc{}
	var order []string
	for _, r := range runs {
		for k, j := range jobTotals(r) {
			a := accs[k]
			if a == nil {
				a = &acc{byCommit: map[string]map[string]bool{}}
				accs[k] = a
				order = append(order, k)
			}
			if !a.seenLatest {
				a.last, a.seenLatest = j.DurationSeconds, true
			}
			a.results = append(a.results, j.Status)
			if j.Status == sampleOK {
				a.durs = append(a.durs, j.DurationSeconds)
				if j.Attempt > 1 {
					a.retriedOK = true
				}
			} else {
				a.failures++
			}
			if r.CommitSHA != "" {
				if a.byCommit[r.CommitSHA] == nil {
					a.byCommit[r.CommitSHA] = map[string]bool{}
				}
				a.byCommit[r.CommitSHA][j.Status] = true
			}
		}
	}
	sort.Strings(order)
	for _, k := range order {
		a := accs[k]
		mixed := false
		for _, st := range a.byCommit {
			if st[sampleOK] && st[sampleFailed] {
				mixed = true
			}
		}
		n := len(a.results)
		fl, why := flaky(a.results, mixed, a.retriedOK)
		res.Jobs = append(res.Jobs, JobInsight{
			Job: k, Runs: n, Failures: a.failures, FailureRate: float64(a.failures) / float64(n),
			P50Seconds: median(a.durs), LastSeconds: a.last, Flaky: fl, FlakyReason: why,
		})
	}
	if len(runs) < 2 {
		return res
	}
	cur := jobTotals(runs[0])
	for _, base := range runs[1:] {
		bt := jobTotals(base)
		ok := len(bt) > 0
		for _, j := range bt {
			if j.Status != sampleOK {
				ok = false
			}
		}
		if !ok {
			continue
		}
		res.DiffRunID, res.DiffBaseID = runs[0].RunID, base.RunID
		keys := make([]string, 0, len(cur))
		for k := range cur {
			if _, shared := bt[k]; shared {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			d := JobDelta{Job: k, BaselineSeconds: bt[k].DurationSeconds, CurrentSeconds: cur[k].DurationSeconds}
			d.DeltaSeconds = d.CurrentSeconds - d.BaselineSeconds
			if d.BaselineSeconds > 0 {
				d.DeltaPercent = float64(d.DeltaSeconds) * 100 / float64(d.BaselineSeconds)
			}
			res.Diff = append(res.Diff, d)
		}
		break
	}
	return res
}
