package build

import (
	"testing"
	"time"
)

func okRun(sec, cached int) Run {
	return Run{Status: RunSuccess, Duration: time.Duration(sec) * time.Second, CachedSteps: cached, TotalSteps: 10}
}

func codes(r SpeedReport) map[string]SpeedFinding {
	m := map[string]SpeedFinding{}
	for _, f := range r.Findings {
		m[f.Code] = f
	}
	return m
}

func TestAdviseSpeed(t *testing.T) {
	cold := []Run{okRun(100, 0), okRun(100, 1), okRun(100, 0)}
	tests := []struct {
		name      string
		runs      []Run
		cache     SpeedCacheState
		want      []string
		wantSaved float64
	}{
		{"no runs", nil, SpeedCacheState{}, nil, 0},
		{"cold unconfigured", cold, SpeedCacheState{}, []string{SpeedCacheUnconfigured, SpeedLowCacheHit}, 0},
		{"cold disabled", cold, SpeedCacheState{Configured: true, TargetID: "t1"}, []string{SpeedCacheDisabled, SpeedLowCacheHit}, 0},
		{"warm saves time", []Run{okRun(10, 9), okRun(10, 10), okRun(100, 0)}, SpeedCacheState{Configured: true, Enabled: true}, nil, 180},
		{"regression", []Run{okRun(60, 5), okRun(60, 5), okRun(60, 5), okRun(20, 5), okRun(20, 5), okRun(20, 5)}, SpeedCacheState{Configured: true, Enabled: true}, []string{SpeedSlowerRecently}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := AdviseSpeed(tc.runs, tc.cache)
			got := codes(rep)
			if len(got) != len(tc.want) {
				t.Fatalf("findings = %v, want %v", rep.Findings, tc.want)
			}
			for _, c := range tc.want {
				if _, ok := got[c]; !ok {
					t.Fatalf("missing %s in %v", c, rep.Findings)
				}
			}
			if rep.TimeSavedSeconds != tc.wantSaved {
				t.Fatalf("saved = %v, want %v", rep.TimeSavedSeconds, tc.wantSaved)
			}
			if f, ok := got[SpeedCacheDisabled]; ok && (f.FixKind != SpeedFixAuto || f.TargetID != "t1") {
				t.Fatalf("bad auto fix: %+v", f)
			}
		})
	}
}
