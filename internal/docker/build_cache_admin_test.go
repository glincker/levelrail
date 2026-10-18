package docker

import (
	"testing"
	"time"
)

func TestPlanBuildCachePrune(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	records := []BuildCacheRecord{
		{ID: "old-idle", SizeBytes: 10, LastUsedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "old-busy", SizeBytes: 20, InUse: true, LastUsedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "fresh", SizeBytes: 30, LastUsedAt: now.Add(-time.Hour)},
		{ID: "never-used", SizeBytes: 40, CreatedAt: now.Add(-30 * 24 * time.Hour)},
	}
	tests := []struct {
		name      string
		olderThan time.Duration
		want      []string
	}{
		{name: "a week keeps fresh and in-use records", olderThan: 7 * 24 * time.Hour, want: []string{"old-idle", "never-used"}},
		{name: "zero selects every idle record", olderThan: 0, want: []string{"old-idle", "fresh", "never-used"}},
		{name: "a year selects nothing", olderThan: 365 * 24 * time.Hour, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanBuildCachePrune(records, tc.olderThan, now)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d records, want %v", len(got), tc.want)
			}
			for i, r := range got {
				if r.ID != tc.want[i] {
					t.Fatalf("record %d = %s, want %s", i, r.ID, tc.want[i])
				}
			}
		})
	}
}
