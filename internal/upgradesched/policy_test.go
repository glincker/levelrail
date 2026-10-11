package upgradesched

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, v string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestWindowNext(t *testing.T) {
	tests := []struct {
		name      string
		win       Window
		now       string
		wantStart string
		wantEnd   string
	}{
		{"before the window opens", Window{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "Europe/Berlin"},
			"2026-10-11T12:00:00Z", "2026-10-12T01:00:00Z", "2026-10-12T03:00:00Z"},
		{"inside the window runs now until it closes", Window{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "Europe/Berlin"},
			"2026-10-12T01:30:00Z", "2026-10-12T01:30:00Z", "2026-10-12T03:00:00Z"},
		{"timezone changes the instant", Window{Cron: "0 3 * * *", Duration: time.Hour, Timezone: "America/Los_Angeles"},
			"2026-10-11T12:00:00Z", "2026-10-12T10:00:00Z", "2026-10-12T11:00:00Z"},
		{"window straddling midnight stays open", Window{Cron: "30 23 * * *", Duration: 3 * time.Hour, Timezone: "UTC"},
			"2026-10-12T01:00:00Z", "2026-10-12T01:00:00Z", "2026-10-12T02:30:00Z"},
		{"fall back keeps the wall clock", Window{Cron: "0 3 * * *", Duration: time.Hour, Timezone: "Europe/Berlin"},
			"2026-10-24T12:00:00Z", "2026-10-25T02:00:00Z", "2026-10-25T03:00:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sp, ok, err := tc.win.Next(mustTime(t, tc.now))
			if err != nil || !ok {
				t.Fatalf("Next: ok=%v err=%v", ok, err)
			}
			if got := sp.Start.UTC().Format(time.RFC3339); got != tc.wantStart {
				t.Errorf("start = %s, want %s", got, tc.wantStart)
			}
			if got := sp.End.UTC().Format(time.RFC3339); got != tc.wantEnd {
				t.Errorf("end = %s, want %s", got, tc.wantEnd)
			}
		})
	}
}

func TestWindowSpringForwardGap(t *testing.T) {
	w := Window{Cron: "30 2 * * *", Duration: time.Hour, Timezone: "America/New_York"}
	sp, ok, err := w.Next(mustTime(t, "2026-03-08T04:00:00Z"))
	if err != nil || !ok {
		t.Fatalf("Next: ok=%v err=%v", ok, err)
	}
	// 02:30 does not exist on 2026-03-08 in New York; the opening must land
	// within the hour either side of the gap, never a day late.
	lo, hi := mustTime(t, "2026-03-08T06:30:00Z"), mustTime(t, "2026-03-08T07:30:00Z")
	if sp.Start.Before(lo) || sp.Start.After(hi) {
		t.Fatalf("start %s outside [%s, %s]", sp.Start, lo, hi)
	}
}

func TestWindowValidate(t *testing.T) {
	tests := []struct {
		name    string
		w       Window
		wantErr bool
	}{
		{"ok", Window{Cron: "0 3 * * *", Duration: time.Hour, Timezone: "UTC"}, false},
		{"bad cron", Window{Cron: "nope", Duration: time.Hour, Timezone: "UTC"}, true},
		{"bad timezone", Window{Cron: "0 3 * * *", Duration: time.Hour, Timezone: "Mars/Olympus"}, true},
		{"too short", Window{Cron: "0 3 * * *", Duration: time.Minute, Timezone: "UTC"}, true},
		{"too long", Window{Cron: "0 3 * * *", Duration: 48 * time.Hour, Timezone: "UTC"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.w.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestPolicyValidate(t *testing.T) {
	win := Window{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "UTC"}
	tests := []struct {
		name    string
		p       Policy
		wantErr bool
	}{
		{"off needs no window", Policy{Mode: ModeOff}, false},
		{"patch with window", Policy{Mode: ModePatch, Window: win, MinReleaseAgeDays: 3}, false},
		{"any without window is refused", Policy{Mode: ModeAny}, true},
		{"unknown mode", Policy{Mode: "yolo", Window: win}, true},
		{"negative age", Policy{Mode: ModeAny, Window: win, MinReleaseAgeDays: -1}, true},
		{"absurd age", Policy{Mode: ModeAny, Window: win, MinReleaseAgeDays: 400}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.p.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestEligible(t *testing.T) {
	now := mustTime(t, "2026-10-11T12:00:00Z")
	win := Window{Cron: "0 3 * * *", Duration: 2 * time.Hour, Timezone: "UTC"}
	old := now.Add(-10 * 24 * time.Hour)
	young := now.Add(-24 * time.Hour)
	tests := []struct {
		name     string
		p        Policy
		current  string
		cand     Candidate
		needAck  int
		notes    bool
		wantOK   bool
		wantCode string
	}{
		{"patch release", Policy{Mode: ModePatch, Window: win}, "v0.5.0", Candidate{"v0.5.2", old}, 0, true, true, VerdictEligible},
		{"patch mode skips a minor", Policy{Mode: ModePatch, Window: win}, "v0.5.0", Candidate{"v0.6.0", old}, 0, true, false, VerdictNotPatch},
		{"any mode takes a minor", Policy{Mode: ModeAny, Window: win}, "v0.5.0", Candidate{"v0.6.0", old}, 0, true, true, VerdictEligible},
		{"never a major jump", Policy{Mode: ModeAny, Window: win}, "v0.9.0", Candidate{"v1.0.0", old}, 0, true, false, VerdictMajor},
		{"unacked breaking is held", Policy{Mode: ModeAny, Window: win}, "v0.5.0", Candidate{"v0.5.1", old}, 1, true, false, VerdictBreaking},
		{"unreadable notes are held", Policy{Mode: ModeAny, Window: win}, "v0.5.0", Candidate{"v0.5.1", old}, 0, false, false, VerdictNoNotes},
		{"minimum release age", Policy{Mode: ModeAny, Window: win, MinReleaseAgeDays: 3}, "v0.5.0", Candidate{"v0.5.1", young}, 0, true, false, VerdictTooYoung},
		{"old enough passes the age gate", Policy{Mode: ModeAny, Window: win, MinReleaseAgeDays: 3}, "v0.5.0", Candidate{"v0.5.1", old}, 0, true, true, VerdictEligible},
		{"zero age means immediately", Policy{Mode: ModeAny, Window: win}, "v0.5.0", Candidate{"v0.5.1", now}, 0, true, true, VerdictEligible},
		{"paused", Policy{Mode: ModeAny, Window: win, Paused: true}, "v0.5.0", Candidate{"v0.5.1", old}, 0, true, false, VerdictPaused},
		{"off", Policy{Mode: ModeOff, Window: win}, "v0.5.0", Candidate{"v0.5.1", old}, 0, true, false, VerdictOff},
		{"no window", Policy{Mode: ModeAny}, "v0.5.0", Candidate{"v0.5.1", old}, 0, true, false, VerdictNoWindow},
		{"not newer", Policy{Mode: ModeAny, Window: win}, "v0.5.1", Candidate{"v0.5.1", old}, 0, true, false, VerdictNotNewer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := Eligible(tc.p, tc.current, tc.cand, tc.needAck, tc.notes, now)
			if v.OK != tc.wantOK || v.Code != tc.wantCode {
				t.Fatalf("verdict = %+v, want ok=%v code=%s", v, tc.wantOK, tc.wantCode)
			}
			if tc.wantCode == VerdictTooYoung && v.WaitUntil.IsZero() {
				t.Error("too young must say when it becomes eligible")
			}
		})
	}
}
