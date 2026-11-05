package setupguide

import (
	"testing"
	"time"
)

func TestRestoreDrillDue(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	ago := func(d int) time.Time { return now.Add(-time.Duration(d) * 24 * time.Hour) }
	const days = 7
	cases := []struct {
		name        string
		backup, ver time.Time
		want        RestoreReason
		due         bool
		daysSince   int
	}{
		{"zero both", time.Time{}, time.Time{}, RestoreNoBackup, false, 0},
		{"zero backup verified set", time.Time{}, ago(1), RestoreNoBackup, false, 0},
		{"now before backup", now.Add(time.Hour), time.Time{}, RestoreNoBackup, false, 0},
		{"never verified", ago(3), time.Time{}, RestoreNeverVerified, true, 3},
		{"fresh verify", ago(10), ago(2), RestoreOK, false, 2},
		{"boundary equals days", ago(20), ago(days), RestoreOK, false, days},
		{"just past boundary", ago(20), ago(days + 1), RestoreVerificationOld, true, days + 1},
		{"future verify", ago(5), now.Add(time.Hour), RestoreOK, false, 0},
	}
	for _, c := range cases {
		got := RestoreDrillDue(c.backup, c.ver, now, days)
		if got.Reason != c.want || got.Due != c.due || got.DaysSince != c.daysSince {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}
