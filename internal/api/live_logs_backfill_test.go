package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type backfillQuerier struct {
	TelemetryQuerier
	lineAge time.Duration
	windows []time.Duration
}

func (b *backfillQuerier) QueryLogs(_ context.Context, _ string, from, to time.Time, _ string) ([]telemetry.LogEntry, error) {
	b.windows = append(b.windows, to.Sub(from))
	if to.Sub(from) >= b.lineAge {
		return []telemetry.LogEntry{{Message: "started"}}, nil
	}
	return nil, nil
}

func TestQueryLiveBackfill_WidensOnlyWhileEmpty(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name        string
		lineAge     time.Duration
		wantWindows int
	}{
		{"recent output stops at the first window", time.Minute, 1},
		{"quiet container widens to an hour", 30 * time.Minute, 2},
		{"startup lines from hours ago still show", 5 * time.Hour, 3},
		{"nothing logged anywhere tries every window once", 365 * 24 * time.Hour, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &backfillQuerier{lineAge: tc.lineAge}
			entries, err := queryLiveBackfill(context.Background(), q, "app:demo", now)
			if err != nil {
				t.Fatalf("queryLiveBackfill() error = %v", err)
			}
			if len(q.windows) != tc.wantWindows {
				t.Errorf("queried %d windows, want %d", len(q.windows), tc.wantWindows)
			}
			if tc.lineAge < 365*24*time.Hour && len(entries) == 0 {
				t.Error("got no entries, want the quiet container's last output")
			}
		})
	}
}
