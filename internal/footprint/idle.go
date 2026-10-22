package footprint

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
)

// Environment variables shared with scripts/idle-footprint, which has no defaults on purpose.
const (
	EnvTolRSSPct     = "IDLE_TOL_RSS_PCT"
	EnvTolGoroutines = "IDLE_TOL_GOROUTINES"
	EnvTolFDs        = "IDLE_TOL_FDS"
	EnvTolCPUPoints  = "IDLE_TOL_CPU_POINTS"
	EnvCPUWindowSecs = "APP_FOOTPRINT_CPU_WINDOW_SECONDS"

	defaultCPUWindow = 60 * time.Second
)

// Assessment statuses.
const (
	StatusWithin       = "within_tolerance"
	StatusOver         = "over_tolerance"
	StatusNoBaseline   = "no_baseline"
	StatusToleranceOff = "tolerance_unset"
)

//go:embed idle_baseline.json
var idleBaselineJSON []byte

// BaselineRow is one committed idle measurement from scripts/idle-footprint-baseline.json.
type BaselineRow struct {
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Apps          int     `json:"apps"`
	AppMode       string  `json:"app_mode"`
	WindowSeconds int     `json:"window_seconds"`
	RSSMB         float64 `json:"rss_mb"`
	PeakRSSMB     float64 `json:"peak_rss_mb"`
	CPUPercent    float64 `json:"cpu_percent"`
	Goroutines    int     `json:"goroutines"`
	OpenFDs       int     `json:"open_fds"`
	Threads       int     `json:"threads"`
	Loops         int     `json:"loops"`
}

var (
	baselineOnce sync.Once
	baselineRows []BaselineRow
)

// Baseline returns the committed idle measurements compiled into the binary.
func Baseline() []BaselineRow {
	baselineOnce.Do(func() { _ = json.Unmarshal(idleBaselineJSON, &baselineRows) })
	return baselineRows
}

// Tolerance is how far a reading may rise above its baseline row.
type Tolerance struct {
	RSSPercent float64 `json:"rss_percent"`
	Goroutines int     `json:"goroutines"`
	FDs        int     `json:"open_fds"`
	CPUPoints  float64 `json:"cpu_points"`
}

// ToleranceFromEnv fails when any variable is unset or invalid: a guard with silent limits is a guard nobody tuned.
func ToleranceFromEnv(get func(string) string) (Tolerance, error) {
	var errs []error
	num := func(name string) float64 {
		v, err := strconv.ParseFloat(get(name), 64)
		if err != nil || v < 0 {
			errs = append(errs, fmt.Errorf("%s must be a non-negative number", name))
			return 0
		}
		return v
	}
	t := Tolerance{RSSPercent: num(EnvTolRSSPct), CPUPoints: num(EnvTolCPUPoints)}
	t.Goroutines, t.FDs = int(num(EnvTolGoroutines)), int(num(EnvTolFDs))
	return t, errors.Join(errs...)
}

// CPUWindowFromEnv returns the CPU averaging window, matching the baseline run's 60 seconds when unset.
func CPUWindowFromEnv(get func(string) string) time.Duration {
	if n, err := strconv.Atoi(get(EnvCPUWindowSecs)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultCPUWindow
}

// SelectBaseline picks the stopped-app row for this platform closest to the live app count.
func SelectBaseline(rows []BaselineRow, goos, goarch string, apps int) (BaselineRow, bool) {
	var best BaselineRow
	found := false
	for _, r := range rows {
		if r.OS != goos || r.Arch != goarch || r.AppMode != "stopped" {
			continue
		}
		if !found || absInt(r.Apps-apps) < absInt(best.Apps-apps) {
			best, found = r, true
		}
	}
	return best, found
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Reading is the live control plane footprint compared against a baseline.
type Reading struct {
	RSSBytes         uint64  `json:"rss_bytes"`
	CPUPercent       float64 `json:"cpu_percent"`
	CPUWindowSeconds float64 `json:"cpu_window_seconds"`
	Goroutines       int     `json:"goroutines"`
	OpenFDs          int     `json:"open_fds"`
	Threads          int     `json:"threads"`
	Apps             int     `json:"apps"`
}

// MetricCheck is one live value next to its baseline and the limit the tolerance allows.
type MetricCheck struct {
	Name     string  `json:"name"`
	Unit     string  `json:"unit"`
	Current  float64 `json:"current"`
	Baseline float64 `json:"baseline"`
	Limit    float64 `json:"limit"`
	OK       bool    `json:"ok"`
}

// Assessment is the live footprint judged against the committed idle baseline.
type Assessment struct {
	Status    string        `json:"status"`
	Detail    string        `json:"detail,omitempty"`
	Platform  string        `json:"platform"`
	Current   Reading       `json:"current"`
	Baseline  *BaselineRow  `json:"baseline,omitempty"`
	Tolerance *Tolerance    `json:"tolerance,omitempty"`
	Checks    []MetricCheck `json:"checks"`
}

// Assess compares a reading with the baseline row for this platform. The baseline records a peak RSS, so live RSS is held to the peak limit.
func Assess(cur Reading, rows []BaselineRow, goos, goarch string, tol Tolerance, tolErr error) Assessment {
	a := Assessment{Current: cur, Checks: []MetricCheck{}, Platform: goos + "/" + goarch}
	row, ok := SelectBaseline(rows, goos, goarch, cur.Apps)
	if !ok {
		a.Status = StatusNoBaseline
		a.Detail = "no committed idle baseline exists for " + a.Platform
		return a
	}
	a.Baseline = &row
	if tolErr != nil {
		a.Status = StatusToleranceOff
		a.Detail = "set " + EnvTolRSSPct + ", " + EnvTolGoroutines + ", " + EnvTolFDs + " and " + EnvTolCPUPoints + " to judge the footprint"
		return a
	}
	a.Tolerance = &tol
	rssMB := float64(cur.RSSBytes) / (1 << 20)
	rssLimit := row.PeakRSSMB * (1 + tol.RSSPercent/100)
	cpuLimit := row.CPUPercent + tol.CPUPoints
	a.Checks = []MetricCheck{
		{Name: "rss", Unit: "MB", Current: round1(rssMB), Baseline: round1(row.PeakRSSMB), Limit: round1(rssLimit), OK: rssMB <= rssLimit},
		{Name: "cpu", Unit: "%", Current: round2(cur.CPUPercent), Baseline: round2(row.CPUPercent), Limit: round2(cpuLimit), OK: cur.CPUPercent <= cpuLimit},
		{Name: "goroutines", Unit: "count", Current: float64(cur.Goroutines), Baseline: float64(row.Goroutines), Limit: float64(row.Goroutines + tol.Goroutines), OK: cur.Goroutines <= row.Goroutines+tol.Goroutines},
		{Name: "open_fds", Unit: "count", Current: float64(cur.OpenFDs), Baseline: float64(row.OpenFDs), Limit: float64(row.OpenFDs + tol.FDs), OK: cur.OpenFDs <= row.OpenFDs+tol.FDs},
	}
	a.Status = StatusWithin
	for _, c := range a.Checks {
		if !c.OK {
			a.Status = StatusOver
			break
		}
	}
	return a
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
