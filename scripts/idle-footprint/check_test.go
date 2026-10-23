package main

import (
	"strings"
	"testing"
)

func base() Result {
	return Result{OS: "linux", Arch: "arm64", Apps: 5, AppMode: modeStopped, RSSMB: 100, PeakRSSMB: 100, CPUPercent: 0.5, Goroutines: 100, OpenFDs: 50}
}

func TestCompare(t *testing.T) {
	tol := Tolerance{RSSPercent: 10, Goroutines: 5, FDs: 3, CPUPoints: 0.2}
	tests := []struct {
		name    string
		mutate  func(*Result)
		wantErr string
	}{
		{"identical", func(*Result) {}, ""},
		{"within tolerance", func(r *Result) { r.PeakRSSMB, r.Goroutines, r.OpenFDs, r.CPUPercent = 110, 105, 53, 0.7 }, ""},
		{"lower is fine", func(r *Result) { r.PeakRSSMB, r.Goroutines = 10, 1 }, ""},
		{"rss over", func(r *Result) { r.PeakRSSMB = 110.5 }, "peak RSS"},
		{"goroutines over", func(r *Result) { r.Goroutines = 106 }, "goroutines"},
		{"fds over", func(r *Result) { r.OpenFDs = 54 }, "open fds"},
		{"cpu over", func(r *Result) { r.CPUPercent = 0.71 }, "CPU"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := base()
			tc.mutate(&got)
			v, missing := Compare([]Result{base()}, []Result{got}, tol)
			if len(missing) != 0 {
				t.Fatalf("unexpected missing rows %v", missing)
			}
			if tc.wantErr == "" && len(v) != 0 {
				t.Fatalf("unexpected violations %v", v)
			}
			if tc.wantErr != "" && (len(v) != 1 || !strings.Contains(v[0], tc.wantErr)) {
				t.Fatalf("want one %q violation, got %v", tc.wantErr, v)
			}
		})
	}
}

func TestCompareMissingBaselineRow(t *testing.T) {
	other := base()
	other.Apps = 20
	v, missing := Compare([]Result{base()}, []Result{other}, Tolerance{})
	if len(v) != 0 || len(missing) != 1 {
		t.Fatalf("want one missing row and no violations, got %v %v", v, missing)
	}
}

func TestToleranceFromEnvRequiresEveryKnob(t *testing.T) {
	env := map[string]string{envTolRSSPct: "10", envTolGoroutines: "5", envTolFDs: "3", envTolCPUPoints: "0.2"}
	tol, err := ToleranceFromEnv(func(k string) string { return env[k] })
	if err != nil || tol.RSSPercent != 10 || tol.Goroutines != 5 || tol.FDs != 3 || tol.CPUPoints != 0.2 {
		t.Fatalf("got %+v, %v", tol, err)
	}
	delete(env, envTolFDs)
	if _, err := ToleranceFromEnv(func(k string) string { return env[k] }); err == nil || !strings.Contains(err.Error(), envTolFDs) {
		t.Fatalf("want an error naming %s, got %v", envTolFDs, err)
	}
	env[envTolFDs] = "-1"
	if _, err := ToleranceFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("negative tolerance must be rejected")
	}
}

func TestMergeResultsReplacesByKey(t *testing.T) {
	old := base()
	nu := base()
	nu.PeakRSSMB = 90
	extra := base()
	extra.Apps = 0
	got := mergeResults([]Result{old}, []Result{nu, extra})
	if len(got) != 2 || got[1].PeakRSSMB != 90 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseCounts(t *testing.T) {
	got, err := parseCounts("0, 5,20")
	if err != nil || len(got) != 3 || got[2] != 20 {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseCounts("0,x"); err == nil {
		t.Fatal("want error")
	}
}
