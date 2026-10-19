// Package soak records control plane resource samples during a long churn run
// and decides pass or fail from growth between the start and the end.
package soak

import (
	"cmp"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"time"
)

// Sample is one reading of the control plane plus the churn counters so far.
type Sample struct {
	At         time.Time
	RSSBytes   uint64
	Goroutines int
	OpenFDs    int
	WALBytes   int64
	Requests   int64
	Errors     int64
}

var csvHeader = []string{"timestamp", "rss_bytes", "goroutines", "open_fds", "wal_bytes", "requests", "errors"}

// WriteCSV writes samples with a header row.
func WriteCSV(w io.Writer, samples []Sample) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, s := range samples {
		row := []string{
			s.At.UTC().Format(time.RFC3339),
			strconv.FormatUint(s.RSSBytes, 10),
			strconv.Itoa(s.Goroutines),
			strconv.Itoa(s.OpenFDs),
			strconv.FormatInt(s.WALBytes, 10),
			strconv.FormatInt(s.Requests, 10),
			strconv.FormatInt(s.Errors, 10),
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// Limits are the ratchet bounds. A zero field disables that check.
type Limits struct {
	RSSGrowthPct    float64 `json:"rss_growth_pct"`
	GoroutineGrowth int     `json:"goroutine_growth"`
	FDGrowth        int     `json:"fd_growth"`
	WALMaxBytes     int64   `json:"wal_max_bytes"`
	ErrorRatePct    float64 `json:"error_rate_pct"`
}

// Summary is the steady state at the start and end of a run, medians over a
// window so one spike does not decide the verdict.
type Summary struct {
	Samples         int     `json:"samples"`
	StartRSSBytes   uint64  `json:"start_rss_bytes"`
	EndRSSBytes     uint64  `json:"end_rss_bytes"`
	StartGoroutines int     `json:"start_goroutines"`
	EndGoroutines   int     `json:"end_goroutines"`
	StartOpenFDs    int     `json:"start_open_fds"`
	EndOpenFDs      int     `json:"end_open_fds"`
	MaxWALBytes     int64   `json:"max_wal_bytes"`
	Requests        int64   `json:"requests"`
	Errors          int64   `json:"errors"`
	ErrorRatePct    float64 `json:"error_rate_pct"`
}

// Result is the verdict of Evaluate.
type Result struct {
	Pass       bool     `json:"pass"`
	Violations []string `json:"violations"`
	Summary    Summary  `json:"summary"`
}

// minSamples is the fewest samples that make a start and end window distinct.
const minSamples = 4

// Summarize reduces samples to start and end medians over the first and last
// quarter of the run.
func Summarize(samples []Sample) Summary {
	sum := Summary{Samples: len(samples)}
	if len(samples) == 0 {
		return sum
	}
	w := len(samples) / 4
	if w < 1 {
		w = 1
	}
	head, tail := samples[:w], samples[len(samples)-w:]
	rss := func(s Sample) uint64 { return s.RSSBytes }
	gor := func(s Sample) int { return s.Goroutines }
	fds := func(s Sample) int { return s.OpenFDs }
	sum.StartRSSBytes, sum.EndRSSBytes = median(head, rss), median(tail, rss)
	sum.StartGoroutines, sum.EndGoroutines = median(head, gor), median(tail, gor)
	sum.StartOpenFDs, sum.EndOpenFDs = median(head, fds), median(tail, fds)
	for _, s := range samples {
		sum.MaxWALBytes = max(sum.MaxWALBytes, s.WALBytes)
	}
	last := samples[len(samples)-1]
	sum.Requests, sum.Errors = last.Requests, last.Errors
	if sum.Requests > 0 {
		sum.ErrorRatePct = float64(sum.Errors) / float64(sum.Requests) * 100
	}
	return sum
}

func median[T cmp.Ordered](in []Sample, f func(Sample) T) T {
	vals := make([]T, 0, len(in))
	for _, s := range in {
		vals = append(vals, f(s))
	}
	slices.Sort(vals)
	return vals[len(vals)/2]
}

// Evaluate applies limits to a run. Too few samples is a failure, because a
// run that never sampled proves nothing.
func Evaluate(samples []Sample, lim Limits) Result {
	sum := Summarize(samples)
	res := Result{Pass: true, Violations: []string{}, Summary: sum}
	fail := func(format string, a ...any) {
		res.Pass = false
		res.Violations = append(res.Violations, fmt.Sprintf(format, a...))
	}
	if len(samples) < minSamples {
		fail("only %d samples recorded, need at least %d", len(samples), minSamples)
		return res
	}
	if lim.RSSGrowthPct > 0 && sum.StartRSSBytes > 0 {
		growth := (float64(sum.EndRSSBytes) - float64(sum.StartRSSBytes)) / float64(sum.StartRSSBytes) * 100
		if growth > lim.RSSGrowthPct {
			fail("RSS grew %.1f%% (%d to %d bytes), limit %.1f%%", growth, sum.StartRSSBytes, sum.EndRSSBytes, lim.RSSGrowthPct)
		}
	}
	if lim.GoroutineGrowth > 0 && sum.EndGoroutines-sum.StartGoroutines > lim.GoroutineGrowth {
		fail("goroutines grew by %d (%d to %d), limit %d", sum.EndGoroutines-sum.StartGoroutines, sum.StartGoroutines, sum.EndGoroutines, lim.GoroutineGrowth)
	}
	if lim.FDGrowth > 0 && sum.EndOpenFDs-sum.StartOpenFDs > lim.FDGrowth {
		fail("open file descriptors grew by %d (%d to %d), limit %d", sum.EndOpenFDs-sum.StartOpenFDs, sum.StartOpenFDs, sum.EndOpenFDs, lim.FDGrowth)
	}
	if lim.WALMaxBytes > 0 && sum.MaxWALBytes > lim.WALMaxBytes {
		fail("WAL reached %d bytes, limit %d", sum.MaxWALBytes, lim.WALMaxBytes)
	}
	if lim.ErrorRatePct > 0 && sum.ErrorRatePct > lim.ErrorRatePct {
		fail("error rate %.2f%%, limit %.2f%%", sum.ErrorRatePct, lim.ErrorRatePct)
	}
	return res
}

// Ratchet is the end state of a previous passing run. A later run fails when
// it ends worse than this by more than the slack, so growth cannot creep in.
type Ratchet struct {
	EndRSSBytes   uint64 `json:"end_rss_bytes"`
	EndGoroutines int    `json:"end_goroutines"`
	EndOpenFDs    int    `json:"end_open_fds"`
}

// LoadRatchet reads a ratchet file. A missing file returns ok false.
func LoadRatchet(path string) (r Ratchet, ok bool, err error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path for a local CLI flag
	if os.IsNotExist(err) {
		return r, false, nil
	}
	if err != nil {
		return r, false, fmt.Errorf("read ratchet: %w", err)
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, false, fmt.Errorf("parse ratchet %s: %w", path, err)
	}
	return r, true, nil
}

// SaveRatchet writes the end state of sum.
func SaveRatchet(path string, sum Summary) error {
	b, err := json.MarshalIndent(Ratchet{EndRSSBytes: sum.EndRSSBytes, EndGoroutines: sum.EndGoroutines, EndOpenFDs: sum.EndOpenFDs}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ratchet: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write ratchet: %w", err)
	}
	return nil
}

// CheckRatchet adds a violation to res when the run ended worse than prev by more than slackPct.
func CheckRatchet(res *Result, prev Ratchet, slackPct float64) {
	over := func(now, was float64) bool { return was > 0 && now > was*(1+slackPct/100) }
	s := res.Summary
	if over(float64(s.EndRSSBytes), float64(prev.EndRSSBytes)) {
		res.Pass = false
		res.Violations = append(res.Violations, fmt.Sprintf("ended at %d RSS bytes, ratchet %d (+%.0f%% slack)", s.EndRSSBytes, prev.EndRSSBytes, slackPct))
	}
	if over(float64(s.EndGoroutines), float64(prev.EndGoroutines)) {
		res.Pass = false
		res.Violations = append(res.Violations, fmt.Sprintf("ended at %d goroutines, ratchet %d (+%.0f%% slack)", s.EndGoroutines, prev.EndGoroutines, slackPct))
	}
	if over(float64(s.EndOpenFDs), float64(prev.EndOpenFDs)) {
		res.Pass = false
		res.Violations = append(res.Violations, fmt.Sprintf("ended at %d open file descriptors, ratchet %d (+%.0f%% slack)", s.EndOpenFDs, prev.EndOpenFDs, slackPct))
	}
}
