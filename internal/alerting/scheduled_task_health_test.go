package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEvaluateScheduledTaskHealth(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	since := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }

	tests := []struct {
		name       string
		kind       Kind
		task       store.ScheduledTask
		threshold  int
		wantFiring bool
		wantNotice string
	}{
		{"missed below threshold", KindScheduledTaskMissed, store.ScheduledTask{ID: "t", Name: "nightly", ConsecutiveMissed: 1}, 2, false, ""},
		{"missed at threshold", KindScheduledTaskMissed, store.ScheduledTask{ID: "t", Name: "nightly", ConsecutiveMissed: 2}, 2, true, "missed 2"},
		{"missed defaults to 1", KindScheduledTaskMissed, store.ScheduledTask{ID: "t", Name: "nightly", ConsecutiveMissed: 1}, 0, true, "nightly"},
		{"not running", KindScheduledTaskRunningLong, store.ScheduledTask{ID: "t", WarnAfterSeconds: 60}, 0, false, ""},
		{"running under warn", KindScheduledTaskRunningLong, store.ScheduledTask{ID: "t", WarnAfterSeconds: 600, RunningSince: since(time.Minute)}, 0, false, ""},
		{"running over warn", KindScheduledTaskRunningLong, store.ScheduledTask{ID: "t", Name: "report", WarnAfterSeconds: 60, RunningSince: since(5 * time.Minute)}, 0, true, "5m0s"},
		{"no warn threshold never fires", KindScheduledTaskRunningLong, store.ScheduledTask{ID: "t", RunningSince: since(time.Hour)}, 0, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newFakeScheduledTaskSource(tt.task)
			r := Rule{ID: "r1", Kind: tt.kind, ScheduledTaskID: "t", RestartCountThreshold: tt.threshold, Enabled: true}
			got, notice, err := EvaluateScheduledTaskHealth(context.Background(), src, r, now)
			if err != nil {
				t.Fatalf("EvaluateScheduledTaskHealth: %v", err)
			}
			if got.Firing != tt.wantFiring {
				t.Errorf("Firing = %v, want %v", got.Firing, tt.wantFiring)
			}
			if !strings.Contains(notice, tt.wantNotice) || (tt.wantNotice == "" && notice != "") {
				t.Errorf("notice = %q, want it to contain %q", notice, tt.wantNotice)
			}
		})
	}
}

func TestEvaluateScheduledTaskHealth_DeletedJobGoesQuiet(t *testing.T) {
	r := Rule{ID: "r1", Kind: KindScheduledTaskMissed, ScheduledTaskID: "gone", Firing: true, Enabled: true}
	got, notice, err := EvaluateScheduledTaskHealth(context.Background(), newFakeScheduledTaskSource(), r, time.Now())
	if err != nil || got.Firing || notice != "" {
		t.Errorf("got Firing=%v notice=%q err=%v, want quiet and no error", got.Firing, notice, err)
	}
}
