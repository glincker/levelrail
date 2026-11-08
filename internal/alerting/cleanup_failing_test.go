package alerting

import (
	"context"
	"testing"
	"time"
)

type fakeCleanup struct{ st CleanupFailures }

func (f fakeCleanup) CleanupFailures(context.Context, time.Time) (CleanupFailures, error) {
	return f.st, nil
}

func TestEvaluateCleanupFailing(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		st        CleanupFailures
		threshold float64
		want      bool
	}{
		{"below default floor", CleanupFailures{Failed: 1, Window: time.Hour, DefaultFloor: 2}, 0, false},
		{"at default floor", CleanupFailures{Failed: 2, Window: time.Hour, DefaultFloor: 2, LastTarget: "audit"}, 0, true},
		{"rule threshold wins", CleanupFailures{Failed: 2, Window: time.Hour, DefaultFloor: 1}, 3, false},
		{"zero floor means one", CleanupFailures{Failed: 1, Window: time.Hour}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Rule{ID: "r", Kind: KindCleanupFailing, Threshold: tc.threshold}
			_, notice, err := EvaluateCleanupFailing(context.Background(), fakeCleanup{tc.st}, r, now)
			if err != nil {
				t.Fatal(err)
			}
			if (notice != "") != tc.want {
				t.Fatalf("notice %q, want firing=%v", notice, tc.want)
			}
		})
	}
}
