package live

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRestoreTracker(t *testing.T) {
	tests := []struct {
		name     string
		counts   []int
		elapsed  []time.Duration
		wantDone bool
		want     restoreOutcome
	}{
		{"dip then recovery", []int{2, 1, 1, 2}, ms3(10, 60, 110, 565), true, restoreOutcome{Restored: true, DropSeen: true, Elapsed: 565 * time.Millisecond}},
		{"still down", []int{1, 1, 1}, ms3(10, 60, 5000), false, restoreOutcome{}},
		{"never dipped before the grace", []int{2, 2}, ms3(10, 1500), false, restoreOutcome{}},
		{"never dipped after the grace", []int{2, 2, 2}, ms3(10, 1500, 2100), true, restoreOutcome{Restored: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := restoreTracker{want: 2}
			var done bool
			var out restoreOutcome
			for i, c := range tt.counts {
				if done, out = tr.observe(c, tt.elapsed[i]); done {
					break
				}
			}
			if done != tt.wantDone || out != tt.want {
				t.Errorf("done=%t out=%+v, want done=%t out=%+v", done, out, tt.wantDone, tt.want)
			}
		})
	}
}

func ms3(v ...int) []time.Duration {
	out := make([]time.Duration, len(v))
	for i, x := range v {
		out[i] = time.Duration(x) * time.Millisecond
	}
	return out
}

func TestWatchRestoreReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out := watchRestore(ctx, func() int { return 1 }, 2, time.Now()); out.Restored {
		t.Errorf("cancelled watch must not report a restore: %+v", out)
	}
}

func TestRestoreDescribe(t *testing.T) {
	if got := (restoreOutcome{Restored: true}).describe(); !strings.Contains(got, "not resolved") {
		t.Errorf("describe = %q", got)
	}
	if got := (restoreOutcome{}).describe(); !strings.Contains(got, "never returned") {
		t.Errorf("describe = %q", got)
	}
	if got := boolMetric(true) + boolMetric(false); got != 1 {
		t.Errorf("boolMetric sum = %v", got)
	}
}
