package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

const (
	envTolRSSPct     = "IDLE_TOL_RSS_PCT"
	envTolGoroutines = "IDLE_TOL_GOROUTINES"
	envTolFDs        = "IDLE_TOL_FDS"
	envTolCPUPoints  = "IDLE_TOL_CPU_POINTS"
)

// Tolerance is how far a measurement may drift above its baseline row.
type Tolerance struct {
	RSSPercent float64
	Goroutines int
	FDs        int
	CPUPoints  float64
}

// ToleranceFromEnv has no defaults on purpose: a guard with silent limits is a guard nobody tuned.
func ToleranceFromEnv(get func(string) string) (Tolerance, error) {
	var t Tolerance
	var errs []error
	num := func(name string) float64 {
		v, err := strconv.ParseFloat(get(name), 64)
		if err != nil || v < 0 {
			errs = append(errs, fmt.Errorf("%s must be a non-negative number, got %q", name, get(name)))
			return 0
		}
		return v
	}
	t.RSSPercent = num(envTolRSSPct)
	t.Goroutines, t.FDs = int(num(envTolGoroutines)), int(num(envTolFDs))
	t.CPUPoints = num(envTolCPUPoints)
	return t, errors.Join(errs...)
}

func key(r Result) string {
	return fmt.Sprintf("%s/%s apps=%d mode=%s", r.OS, r.Arch, r.Apps, r.AppMode)
}

// Compare returns one violation per metric that grew past its tolerance. Rows with no baseline are returned as missing, not as failures.
func Compare(baseline, got []Result, tol Tolerance) (violations, missing []string) {
	want := make(map[string]Result, len(baseline))
	for _, b := range baseline {
		want[key(b)] = b
	}
	for _, g := range got {
		b, ok := want[key(g)]
		if !ok {
			missing = append(missing, key(g))
			continue
		}
		if limit := b.PeakRSSMB * (1 + tol.RSSPercent/100); g.PeakRSSMB > limit {
			violations = append(violations, fmt.Sprintf("%s: peak RSS %.1f MB over limit %.1f MB (baseline %.1f)", key(g), g.PeakRSSMB, limit, b.PeakRSSMB))
		}
		if g.Goroutines > b.Goroutines+tol.Goroutines {
			violations = append(violations, fmt.Sprintf("%s: goroutines %d over baseline %d + %d", key(g), g.Goroutines, b.Goroutines, tol.Goroutines))
		}
		if g.OpenFDs > b.OpenFDs+tol.FDs {
			violations = append(violations, fmt.Sprintf("%s: open fds %d over baseline %d + %d", key(g), g.OpenFDs, b.OpenFDs, tol.FDs))
		}
		if g.CPUPercent > b.CPUPercent+tol.CPUPoints {
			violations = append(violations, fmt.Sprintf("%s: CPU %.2f%% over baseline %.2f%% + %.2f points", key(g), g.CPUPercent, b.CPUPercent, tol.CPUPoints))
		}
	}
	return violations, missing
}

func checkFiles(baselinePath, resultPath string, tol Tolerance) error {
	base, err := readResults(baselinePath)
	if err != nil {
		return err
	}
	got, err := readResults(resultPath)
	if err != nil {
		return err
	}
	violations, missing := Compare(base, got, tol)
	for _, m := range missing {
		fmt.Fprintf(os.Stderr, "no baseline row for %s, record one with the record subcommand\n", m)
	}
	if len(missing) == len(got) {
		return errors.New("no result row matched a baseline row, nothing was checked")
	}
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, "REGRESSION "+v)
		}
		return fmt.Errorf("%d idle footprint regression(s)", len(violations))
	}
	return nil
}
