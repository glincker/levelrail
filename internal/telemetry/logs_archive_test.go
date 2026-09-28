package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestStreamLogs(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	var entries []LogEntry
	for _, off := range []int{0, 0, 1, 2, 2, 3} {
		entries = append(entries, LogEntry{ResourceID: "service:a", Stream: "stdout", Timestamp: base.Add(time.Duration(off) * time.Second), Message: "m"})
	}
	if err := db.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	from, to := base.UnixNano(), base.Add(time.Minute).UnixNano()

	tests := []struct {
		name      string
		maxLines  int
		wantLines int64
		wantEnd   int64
	}{
		{name: "under cap covers whole range", maxLines: 10, wantLines: 6, wantEnd: to},
		{name: "cap cuts at timestamp boundary", maxLines: 3, wantLines: 3, wantEnd: base.Add(2 * time.Second).UnixNano()},
		{name: "cap smaller than one timestamp group keeps the group", maxLines: 1, wantLines: 2, wantEnd: base.Add(time.Nanosecond).UnixNano()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got int64
			res, err := db.StreamLogs(context.Background(), "service:a", from, to, tt.maxLines, func(LogEntry) error { got++; return nil })
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.wantLines || res.Lines != tt.wantLines || res.EndNs != tt.wantEnd {
				t.Fatalf("lines=%d res=%+v, want lines %d end %d", got, res, tt.wantLines, tt.wantEnd)
			}
		})
	}

	ids, err := db.DistinctLogResources(context.Background(), from, to)
	if err != nil || len(ids) != 1 || ids[0] != "service:a" {
		t.Fatalf("DistinctLogResources = %v, %v", ids, err)
	}
}
