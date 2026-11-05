package api

import (
	"testing"
	"time"
)

func TestInUpdateWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name    string
		cron    string
		minutes int
		now     time.Time
		want    bool
	}{
		{"empty is always open", "", 0, at(12, 0), true},
		{"inside", "0 3 * * *", 60, at(3, 30), true},
		{"at start", "0 3 * * *", 60, at(3, 0), true},
		{"at end is closed", "0 3 * * *", 60, at(4, 0), false},
		{"outside", "0 3 * * *", 60, at(12, 0), false},
	}
	for _, tc := range cases {
		if got := inUpdateWindow(tc.cron, tc.minutes, tc.now); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestResolveUpdateWindow(t *testing.T) {
	cron, bad, mins, neg := "0 3 * * *", "0 0 31 2 *", 30, -1
	if _, _, err := resolveUpdateWindow(setImageAutoUpdateRequest{WindowCron: &bad, WindowMinutes: &mins}); err == nil {
		t.Error("never-matching cron accepted")
	}
	if _, _, err := resolveUpdateWindow(setImageAutoUpdateRequest{WindowCron: &cron, WindowMinutes: &neg}); err == nil {
		t.Error("negative minutes accepted")
	}
	if c, m, err := resolveUpdateWindow(setImageAutoUpdateRequest{WindowCron: &cron, WindowMinutes: &mins}); err != nil || c != cron || m != 30 {
		t.Errorf("got %q %d %v", c, m, err)
	}
}
