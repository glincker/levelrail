package store

import "testing"

func TestComputeWakeTiming(t *testing.T) {
	var many []WakeSample
	for i := 0; i < WakeTimingSamples+25; i++ {
		ms := int64(1000)
		if i >= WakeTimingSamples {
			ms = 999_000
		}
		many = append(many, WakeSample{ReadyMs: ms})
	}
	tests := []struct {
		name         string
		in           []WakeSample
		samples      int
		last, p50    int64
		p95          int64
		releasedLast *int64
	}{
		{"single sample", []WakeSample{{ReadyMs: 3000, ReleasedMs: i64(3200)}}, 1, 3000, 3000, 3000, i64(3200)},
		{"newest first, nearest rank", []WakeSample{{ReadyMs: 9000}, {ReadyMs: 1000}, {ReadyMs: 2000}, {ReadyMs: 3000}}, 4, 9000, 2000, 9000, nil},
		{"history is bounded to the newest samples", many, WakeTimingSamples, 1000, 1000, 1000, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeWakeTiming(tc.in)
			if got.Samples != tc.samples || *got.ReadyLastMs != tc.last || *got.ReadyP50Ms != tc.p50 || *got.ReadyP95Ms != tc.p95 {
				t.Fatalf("timing = %+v, want samples %d last %d p50 %d p95 %d", got, tc.samples, tc.last, tc.p50, tc.p95)
			}
			if (got.ReleasedLastMs == nil) != (tc.releasedLast == nil) || (tc.releasedLast != nil && *got.ReleasedLastMs != *tc.releasedLast) {
				t.Errorf("released last = %v, want %v", got.ReleasedLastMs, tc.releasedLast)
			}
		})
	}
}

func TestComputeWakeTimingNoHistory(t *testing.T) {
	got := ComputeWakeTiming(nil)
	if got.Samples != 0 || got.ReadyLastMs != nil || got.ReadyP50Ms != nil || got.ReadyP95Ms != nil || got.ReleasedP95Ms != nil {
		t.Fatalf("no history must carry no numbers: %+v", got)
	}
}
