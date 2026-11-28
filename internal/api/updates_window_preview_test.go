package api

import (
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/upgradesched"
)

func TestWindowPreview(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		w         upgradesched.Window
		wantFirst string
		wantText  string
	}{
		{name: "daily utc", w: upgradesched.Window{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "UTC"},
			wantFirst: "2026-11-21T03:00:00Z", wantText: "Updates install between 03:00 and 05:00 UTC; backup first, health check, automatic rollback."},
		{name: "zone shifts local time", w: upgradesched.Window{Cron: "0 3 * * *", Duration: time.Hour, Timezone: "Europe/Berlin"},
			wantFirst: "2026-11-21T02:00:00Z", wantText: "Updates install between 03:00 and 04:00 CET; backup first, health check, automatic rollback."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextWindowStarts(tc.w, now, 3)
			if len(got) != 3 || got[0].Start != tc.wantFirst {
				t.Fatalf("got %+v", got)
			}
			if got[1].Start <= got[0].Start || got[2].Start <= got[1].Start {
				t.Fatalf("starts not increasing: %+v", got)
			}
			if s := windowSummary(tc.w, now); s != tc.wantText {
				t.Fatalf("summary = %q", s)
			}
		})
	}
}
