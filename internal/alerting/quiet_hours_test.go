package alerting

import (
	"context"
	"testing"
	"time"
)

func TestQuietHours_ActiveAt(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 9, 25, h, m, 0, 0, time.UTC) }
	tests := []struct {
		name string
		q    QuietHours
		now  time.Time
		want bool
	}{
		{"inside same day", QuietHours{Start: "09:00", End: "17:00", Enabled: true}, at(12, 0), true},
		{"before same day", QuietHours{Start: "09:00", End: "17:00", Enabled: true}, at(8, 59), false},
		{"end is exclusive", QuietHours{Start: "09:00", End: "17:00", Enabled: true}, at(17, 0), false},
		{"overnight late", QuietHours{Start: "22:00", End: "07:00", Enabled: true}, at(23, 30), true},
		{"overnight early", QuietHours{Start: "22:00", End: "07:00", Enabled: true}, at(6, 59), true},
		{"overnight day", QuietHours{Start: "22:00", End: "07:00", Enabled: true}, at(12, 0), false},
		{"disabled", QuietHours{Start: "00:00", End: "23:59"}, at(12, 0), false},
		{"timezone shift", QuietHours{Start: "22:00", End: "07:00", Timezone: "Asia/Kolkata", Enabled: true}, at(17, 0), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.ActiveAt(tt.now); got != tt.want {
				t.Errorf("ActiveAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQuietHours_Validate(t *testing.T) {
	for _, q := range []QuietHours{{Start: "25:00", End: "07:00"}, {Start: "22:00", End: "22:00"}, {Start: "22:00", End: "07:00", Timezone: "Mars/Base"}} {
		if q.Validate() == nil {
			t.Errorf("Validate(%+v) = nil, want an error", q)
		}
	}
	if err := (QuietHours{Start: "22:00", End: "07:00", Timezone: "Europe/Berlin"}).Validate(); err != nil {
		t.Errorf("valid quiet hours rejected: %v", err)
	}
}

func TestRoute_QuietHoursHoldsNonCriticalThenReleases(t *testing.T) {
	ctx := context.Background()
	nc, db, spy := newRouteFixture(t, NoiseConfig{}, nil)
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_q", Name: "ops", Kind: NotifyGeneric, NotifyURL: "https://example.com/h", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetChannelQuietHours(ctx, QuietHours{ChannelID: "chn_q", Start: "11:00", End: "13:00", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	warn := webRule("r1")
	warn.ChannelID = "chn_q"
	crit := webRule("r2")
	crit.ChannelID = "chn_q"
	crit.Severity = SeverityCritical

	nc.Route(ctx, Event{Rule: warn}, t0, spy.send)
	if len(spy.events) != 0 {
		t.Fatal("a warning must be held during quiet hours")
	}
	nc.Route(ctx, Event{Rule: crit}, t0, spy.send)
	if len(spy.events) != 1 || spy.events[0].Rule.ID != "r2" {
		t.Fatalf("a critical alert must always notify, got %+v", spy.events)
	}

	nc.Sweep(ctx, t0.Add(30*time.Minute), spy.send)
	if len(spy.events) != 1 {
		t.Fatal("must stay held while quiet hours are active")
	}
	nc.Sweep(ctx, t0.Add(2*time.Hour), spy.send)
	if len(spy.events) != 2 || spy.events[1].Rule.ID != "r1" {
		t.Fatalf("held warning must be released after quiet hours, got %+v", spy.events)
	}

	var quiet bool
	for _, h := range history(t, db) {
		if h.Outcome == OutcomeQuiet {
			quiet = true
		}
	}
	if !quiet {
		t.Error("expected a quiet_hours history entry")
	}
}
