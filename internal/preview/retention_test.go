package preview

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

func rec(app, id string, age time.Duration, bytes int64, now time.Time) Record {
	return Record{DeploymentID: id, App: app, Status: StatusOK, Bytes: bytes, CapturedAt: now.Add(-age)}
}

func evictedIDs(ev []Eviction) []string {
	var out []string
	for _, e := range ev {
		out = append(out, e.Record.DeploymentID+":"+e.Reason)
	}
	sort.Strings(out)
	return out
}

func TestPlanEviction(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	seven := func(app string) []Record {
		var out []Record
		for i := 0; i < 7; i++ {
			out = append(out, rec(app, fmt.Sprintf("%s%d", app, i), time.Duration(i)*day, 100, now))
		}
		return out
	}
	tests := []struct {
		name      string
		records   []Record
		protected map[string]bool
		policy    RetentionPolicy
		want      []string
	}{
		{
			name:    "under the limit keeps everything",
			records: seven("a")[:3],
			policy:  RetentionPolicy{KeepPerApp: 5, TTL: 30 * day},
			want:    nil,
		},
		{
			name:    "keeps the newest N and evicts older",
			records: seven("a"),
			policy:  RetentionPolicy{KeepPerApp: 5, TTL: 30 * day},
			want:    []string{"a5:keep_limit", "a6:keep_limit"},
		},
		{
			name:      "protected production deployment survives and does not count against N",
			records:   seven("a"),
			protected: map[string]bool{"a6": true},
			policy:    RetentionPolicy{KeepPerApp: 5, TTL: 30 * day},
			want:      []string{"a5:keep_limit"},
		},
		{
			name:    "per app limits are independent",
			records: append(seven("a")[:4], seven("b")[:6]...),
			policy:  RetentionPolicy{KeepPerApp: 5, TTL: 30 * day},
			want:    []string{"b5:keep_limit"},
		},
		{
			name:    "ttl evicts old rows even under the limit",
			records: []Record{rec("a", "old", 40*day, 100, now), rec("a", "new", day, 100, now)},
			policy:  RetentionPolicy{KeepPerApp: 5, TTL: 30 * day},
			want:    []string{"old:ttl"},
		},
		{
			name:      "ttl spares the protected deployment",
			records:   []Record{rec("a", "old", 40*day, 100, now)},
			protected: map[string]bool{"old": true},
			policy:    RetentionPolicy{KeepPerApp: 5, TTL: 30 * day},
			want:      nil,
		},
		{
			name: "budget evicts least recently used first",
			records: []Record{
				rec("a", "a1", 1*day, 100, now),
				rec("b", "b1", 2*day, 100, now),
				rec("c", "c1", 3*day, 100, now),
			},
			policy: RetentionPolicy{KeepPerApp: 5, TTL: 30 * day, MaxTotalBytes: 250},
			want:   []string{"c1:budget"},
		},
		{
			name: "a recent view protects a row from LRU eviction",
			records: func() []Record {
				old := rec("c", "c1", 3*day, 100, now)
				old.LastViewedAt = now
				return []Record{rec("a", "a1", 1*day, 100, now), rec("b", "b1", 2*day, 100, now), old}
			}(),
			policy: RetentionPolicy{KeepPerApp: 5, TTL: 30 * day, MaxTotalBytes: 250},
			want:   []string{"b1:budget"},
		},
		{
			name: "budget never evicts protected rows even when still over",
			records: []Record{
				rec("a", "a1", 1*day, 300, now),
				rec("b", "b1", 2*day, 300, now),
			},
			protected: map[string]bool{"a1": true, "b1": true},
			policy:    RetentionPolicy{KeepPerApp: 5, TTL: 30 * day, MaxTotalBytes: 100},
			want:      nil,
		},
		{
			name: "rows already evicted by the keep limit do not count toward the budget",
			records: []Record{
				rec("a", "a0", 0, 100, now), rec("a", "a1", day, 100, now), rec("a", "a2", 2*day, 100, now),
			},
			policy: RetentionPolicy{KeepPerApp: 2, TTL: 30 * day, MaxTotalBytes: 200},
			want:   []string{"a2:keep_limit"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evictedIDs(PlanEviction(tt.records, tt.protected, now, tt.policy))
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("evictions = %v, want %v", got, want)
			}
		})
	}
}
