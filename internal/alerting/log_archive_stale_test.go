package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeLogArchive []LogArchiveHealth

func (f fakeLogArchive) LogArchiveHealth(context.Context) ([]LogArchiveHealth, error) { return f, nil }

func TestEvaluateLogArchiveStale(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		policies   []LogArchiveHealth
		forDur     time.Duration
		wantFiring bool
		wantNotice string
	}{
		{name: "no policies", wantFiring: false},
		{name: "fresh success", policies: []LogArchiveHealth{{Enabled: true, Interval: time.Hour, CreatedAt: now.Add(-48 * time.Hour), LastSuccessAt: now.Add(-30 * time.Minute)}}},
		{name: "last run failed", policies: []LogArchiveHealth{{Scope: "web", Enabled: true, Interval: time.Hour, LastSuccessAt: now.Add(-time.Minute), LastError: "access denied"}}, wantFiring: true, wantNotice: "web: last run failed"},
		{name: "stale beyond three intervals", policies: []LogArchiveHealth{{Enabled: true, Interval: 2 * time.Hour, CreatedAt: now.Add(-48 * time.Hour), LastSuccessAt: now.Add(-7 * time.Hour)}}, wantFiring: true, wantNotice: "all apps: no successful archive"},
		{name: "never succeeded but young", policies: []LogArchiveHealth{{Enabled: true, Interval: time.Hour, CreatedAt: now.Add(-time.Hour)}}},
		{name: "never succeeded and old", policies: []LogArchiveHealth{{Scope: "api", Enabled: true, Interval: time.Hour, CreatedAt: now.Add(-10 * time.Hour)}}, wantFiring: true, wantNotice: "api: no successful archive"},
		{name: "disabled ignored", policies: []LogArchiveHealth{{Enabled: false, Interval: time.Hour, CreatedAt: now.Add(-100 * time.Hour), LastError: "x"}}},
		{name: "rule override tightens", policies: []LogArchiveHealth{{Enabled: true, Interval: time.Hour, CreatedAt: now.Add(-48 * time.Hour), LastSuccessAt: now.Add(-30 * time.Minute)}}, forDur: 10 * time.Minute, wantFiring: true, wantNotice: "limit 10m0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{ID: "r1", Kind: KindLogArchiveStale, ForDuration: tt.forDur}
			next, notice, err := EvaluateLogArchiveStale(context.Background(), fakeLogArchive(tt.policies), r, now)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if next.Firing != tt.wantFiring {
				t.Fatalf("firing = %v, want %v (notice %q)", next.Firing, tt.wantFiring, notice)
			}
			if !strings.Contains(notice, tt.wantNotice) {
				t.Fatalf("notice %q missing %q", notice, tt.wantNotice)
			}
		})
	}
}
