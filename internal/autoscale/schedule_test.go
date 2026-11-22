package autoscale

import (
	"testing"
	"time"
)

func TestScheduledFloor(t *testing.T) {
	berlin := "Europe/Berlin"
	weekday := Schedule{Days: []string{"mon", "tue", "wed", "thu", "fri"}, Start: "08:00", End: "20:00", Timezone: berlin, MinReplicas: 4}
	night := Schedule{Days: []string{"fri"}, Start: "22:00", End: "02:00", MinReplicas: 6}
	p := testPolicy()
	p.Schedules = []Schedule{weekday, night}
	if err := p.Validate(0); err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tests := []struct {
		name string
		now  time.Time
		want int
	}{
		{"weekday morning in the zone", at("2026-10-19T07:30:00Z"), 4},
		{"before the window opens", at("2026-10-19T05:30:00Z"), 2},
		{"after the window closes", at("2026-10-19T18:30:00Z"), 2},
		{"weekend stays at the base", at("2026-10-17T10:00:00Z"), 2},
		{"overnight window on its start day", at("2026-10-23T23:00:00Z"), 6},
		{"overnight window spills to the next day", at("2026-10-24T01:00:00Z"), 6},
		{"overnight window ends", at("2026-10-24T03:00:00Z"), 2},
	}
	for _, tc := range tests {
		if got := p.EffectiveMin(tc.now); got != tc.want {
			t.Errorf("%s: floor %d, want %d", tc.name, got, tc.want)
		}
	}

	in := Input{Now: at("2026-10-19T08:00:00Z"), Current: 2, Value: 60, ValueKnown: true}
	if d := Decide(p, in); d.Desired != 4 || d.Reason != ReasonBoundsCorrected {
		t.Fatalf("inside the window: desired %d reason %s, want 4 BoundsCorrected", d.Desired, d.Reason)
	}
	in.Now = at("2026-10-19T19:00:00Z")
	if d := Decide(p, in); d.Desired != 2 {
		t.Fatalf("after the window: desired %d, want 2 with the policy intact", d.Desired)
	}

	bad := p
	bad.Schedules = []Schedule{{Start: "08:00", End: "08:00", MinReplicas: 3}}
	if bad.Validate(0) == nil {
		t.Fatal("equal start and end must be rejected")
	}
	bad.Schedules = []Schedule{{Start: "08:00", End: "09:00", MinReplicas: 99}}
	if bad.Validate(0) == nil {
		t.Fatal("a floor above max_replicas must be rejected")
	}
}
