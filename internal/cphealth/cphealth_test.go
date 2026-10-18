package cphealth

import (
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	th := DefaultThresholds(func(string) string { return "" })
	now := time.Now()
	tests := []struct {
		name     string
		in       Inputs
		wantID   string
		wantSev  Severity
		wantFix  string
		wantNone bool
	}{
		{name: "healthy", in: Inputs{Goroutines: 100, Now: now}, wantNone: true},
		{name: "goroutines crit", in: Inputs{Goroutines: 11000, Now: now}, wantID: FindingGoroutines, wantSev: SeverityCrit},
		{name: "wal fixable", in: Inputs{WALBytes: 200 << 20, Now: now}, wantID: FindingWAL, wantSev: SeverityWarn, wantFix: FixWALCheckpoint},
		{name: "db latency", in: Inputs{DBP95MS: 150, Now: now}, wantID: FindingDBLatency, wantSev: SeverityWarn, wantFix: FixDBOptimize},
		{name: "event lag", in: Inputs{ResyncInterval: 30 * time.Second, LastPassAt: now.Add(-5 * time.Minute), Now: now}, wantID: FindingEventLag, wantSev: SeverityCrit},
		{name: "streams near cap", in: Inputs{Streams: 85, StreamCap: 100, Now: now}, wantID: FindingStreams, wantSev: SeverityWarn},
		{name: "streams under cap", in: Inputs{Streams: 10, StreamCap: 100, Now: now}, wantNone: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in, th)
			if tc.wantNone {
				if len(got) != 0 {
					t.Fatalf("want no findings, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].ID != tc.wantID || got[0].Severity != tc.wantSev || got[0].Fix != tc.wantFix {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestDefaultThresholdsEnv(t *testing.T) {
	th := DefaultThresholds(func(k string) string {
		if k == "APP_CPHEALTH_GOROUTINES" {
			return "42"
		}
		return ""
	})
	if th.Goroutines != 42 {
		t.Fatalf("got %d", th.Goroutines)
	}
}
