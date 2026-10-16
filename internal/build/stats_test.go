package build

import (
	"math"
	"testing"
	"time"
)

func ok(sec float64, cached, total int) Run {
	return Run{Status: RunSuccess, Duration: time.Duration(sec * float64(time.Second)), CachedSteps: cached, TotalSteps: total}
}

func TestComputeTrend(t *testing.T) {
	tests := []struct {
		name string
		runs []Run
		want Trend
	}{
		{name: "zero builds", want: Trend{}},
		{
			name: "one build",
			runs: []Run{ok(12, 3, 4)},
			want: Trend{Builds: 1, Succeeded: 1, P50Seconds: 12, P95Seconds: 12, MinSeconds: 12, MaxSeconds: 12, CacheHitRatio: 0.75, CacheKnown: true},
		},
		{
			name: "failed builds are counted but not timed",
			runs: []Run{ok(10, 0, 0), {Status: RunFailed, Duration: 500 * time.Second}, {Status: RunTimeout, Duration: time.Hour}, {Status: RunCanceled}},
			want: Trend{Builds: 4, Succeeded: 1, Failed: 1, TimedOut: 1, Canceled: 1, P50Seconds: 10, P95Seconds: 10, MinSeconds: 10, MaxSeconds: 10},
		},
		{
			name: "negative duration is flagged as skew and not timed",
			runs: []Run{ok(10, 1, 2), {Status: RunSuccess, Duration: -3 * time.Second, CachedSteps: 9, TotalSteps: 9}},
			want: Trend{Builds: 2, Succeeded: 2, Skewed: 1, P50Seconds: 10, P95Seconds: 10, MinSeconds: 10, MaxSeconds: 10, CacheHitRatio: 0.5, CacheKnown: true},
		},
		{
			name: "interpolated percentiles over five builds",
			runs: []Run{ok(50, 0, 0), ok(10, 0, 0), ok(30, 0, 0), ok(20, 0, 0), ok(40, 0, 0)},
			want: Trend{Builds: 5, Succeeded: 5, P50Seconds: 30, P95Seconds: 48, MinSeconds: 10, MaxSeconds: 50},
		},
		{
			name: "only failures leaves timings zero",
			runs: []Run{{Status: RunFailed}, {Status: RunFailed}},
			want: Trend{Builds: 2, Failed: 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeTrend(tc.runs)
			if got.Builds != tc.want.Builds || got.Succeeded != tc.want.Succeeded || got.Failed != tc.want.Failed ||
				got.TimedOut != tc.want.TimedOut || got.Canceled != tc.want.Canceled || got.Skewed != tc.want.Skewed ||
				got.CacheKnown != tc.want.CacheKnown {
				t.Fatalf("counts: got %+v want %+v", got, tc.want)
			}
			for _, p := range [][2]float64{{got.P50Seconds, tc.want.P50Seconds}, {got.P95Seconds, tc.want.P95Seconds},
				{got.MinSeconds, tc.want.MinSeconds}, {got.MaxSeconds, tc.want.MaxSeconds}, {got.CacheHitRatio, tc.want.CacheHitRatio}} {
				if math.Abs(p[0]-p[1]) > 1e-9 {
					t.Fatalf("got %+v want %+v", got, tc.want)
				}
			}
		})
	}
}

func TestAppFromTag(t *testing.T) {
	tests := map[string]string{
		"web:abc":                "web",
		"ghcr.io/org/web:abc":    "web",
		"localhost:5000/web:abc": "web",
		"localhost:5000/web":     "web",
		"web":                    "web",
	}
	for in, want := range tests {
		if got := AppFromTag(in); got != want {
			t.Errorf("AppFromTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStepCounterDedupesAndIgnoresFailures(t *testing.T) {
	var s stepCounter
	s.observe(ProgressEvent{Step: "a", Completed: true, Cached: true})
	s.observe(ProgressEvent{Step: "a", Completed: true, Cached: true})
	s.observe(ProgressEvent{Step: "b", Completed: true})
	s.observe(ProgressEvent{Step: "c", Completed: true, Error: "boom"})
	s.observe(ProgressEvent{Step: "d"})
	if s.total() != 2 || s.cached != 1 {
		t.Fatalf("total=%d cached=%d", s.total(), s.cached)
	}
}
