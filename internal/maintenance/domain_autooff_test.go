package maintenance

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func dur(d time.Duration) *time.Duration { return &d }

func TestResolveAutoOff(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	until := now.Add(time.Hour)
	tests := []struct {
		name    string
		until   *time.Time
		req     *time.Duration
		def     time.Duration
		want    time.Time
		wantErr bool
	}{
		{"env default applies", nil, nil, 2 * time.Hour, now.Add(2 * time.Hour), false},
		{"explicit overrides default", nil, dur(time.Hour), 2 * time.Hour, now.Add(time.Hour), false},
		{"explicit zero opts out", nil, dur(0), 2 * time.Hour, time.Time{}, false},
		{"no default no request", nil, nil, 0, time.Time{}, false},
		{"dated end needs no cut-off", &until, nil, 2 * time.Hour, time.Time{}, false},
		{"negative", nil, dur(-time.Second), 0, time.Time{}, true},
		{"too long", nil, dur(MaxDuration + time.Second), 0, time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveAutoOff(now, tc.until, tc.req, tc.def)
			if (err != nil) != tc.wantErr || !got.Equal(tc.want) {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

func TestAutoOffExpiryWithFakeClock(t *testing.T) {
	start := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	m := store.DomainMaintenance{Domain: "a.example", StartedAt: start, AutoOffAt: start.Add(time.Hour)}
	tests := []struct {
		name   string
		now    time.Time
		active bool
	}{
		{"before cut-off", start.Add(30 * time.Minute), true},
		{"at cut-off", start.Add(time.Hour), false},
		{"control plane was down for a day", start.Add(24 * time.Hour), false},
		{"clock stepped back", start.Add(-time.Hour), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.ActiveAt(tc.now); got != tc.active {
				t.Fatalf("ActiveAt = %v", got)
			}
		})
	}
	if (store.DomainMaintenance{}).ActiveAt(start) != true {
		t.Fatal("open ended maintenance must stay on")
	}
}

func TestKeepStartToggledTwice(t *testing.T) {
	start := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	later := start.Add(3 * time.Hour)
	prev := store.DomainMaintenance{StartedAt: start}
	if got := KeepStart(prev, true, later); !got.Equal(start) {
		t.Fatalf("re-save reset start to %v", got)
	}
	if got := KeepStart(prev, false, later); !got.Equal(later) {
		t.Fatalf("fresh enable should start now, got %v", got)
	}
	if got := KeepStart(store.DomainMaintenance{}, true, later); !got.Equal(later) {
		t.Fatalf("unknown start should become now, got %v", got)
	}
}

func TestElapsedAndOverdue(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if Elapsed(now.Add(time.Hour), now) != 0 {
		t.Fatal("future start (clock skew) must clamp to zero")
	}
	if Elapsed(time.Time{}, now) != 0 {
		t.Fatal("unknown start must be zero")
	}
	rows := []store.DomainMaintenance{
		{Domain: "old", StartedAt: now.Add(-5 * time.Hour)},
		{Domain: "new", StartedAt: now.Add(-time.Hour)},
		{Domain: "skewed", StartedAt: now.Add(time.Hour)},
		{Domain: "unknown"},
	}
	got := Overdue(rows, now, 4*time.Hour)
	if len(got) != 1 || got[0].Domain != "old" {
		t.Fatalf("overdue = %+v", got)
	}
	if Overdue(rows, now, 0) != nil {
		t.Fatal("zero threshold disables the check")
	}
}
