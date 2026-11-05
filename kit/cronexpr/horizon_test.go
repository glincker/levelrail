package cronexpr

import (
	"strings"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("tzdata missing: %v", err)
	}
	return loc
}

func TestNext_ImpossibleDatesAreZero(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, expr := range []string{"0 0 30 2 *", "0 0 31 2 *", "0 0 31 4 *", "0 0 31 4,6,9,11 *"} {
		s, err := Parse(expr)
		if err != nil {
			t.Fatal(err)
		}
		if !s.Next(now).IsZero() {
			t.Errorf("%s: Next not zero", expr)
		}
		for _, name := range []string{"UTC", "America/New_York", "Asia/Kolkata"} {
			if got := NextInLocation(s, now, mustLoc(t, name)); !got.IsZero() {
				t.Errorf("%s in %s: want zero, got %v", expr, name, got)
			}
		}
	}
}

func TestNext_LeapDay(t *testing.T) {
	s, _ := Parse("0 0 29 2 *")
	cases := []struct{ after, want time.Time }{
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)},
		{time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC), time.Date(2032, 2, 29, 0, 0, 0, 0, time.UTC)},
		{time.Date(2028, 2, 29, 23, 59, 0, 0, time.UTC), time.Date(2032, 2, 29, 0, 0, 0, 0, time.UTC)},
		{time.Date(2096, 2, 29, 0, 0, 0, 0, time.UTC), time.Date(2104, 2, 29, 0, 0, 0, 0, time.UTC)},
		{time.Date(2097, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2104, 2, 29, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := s.Next(c.after); !got.Equal(c.want) {
			t.Errorf("Next(%v) = %v, want %v", c.after, got, c.want)
		}
	}
}

func TestNextInLocation_DST(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	lon := mustLoc(t, "Europe/London")
	fixed := time.FixedZone("zero", 0)

	s130, _ := Parse("30 1 * * *")
	after := time.Date(2026, 11, 1, 1, 20, 0, 0, time.FixedZone("EST", -5*3600))
	got := NextInLocation(s130, after, ny)
	if !got.After(after) || !got.Equal(time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)) {
		t.Errorf("fall back NY: got %v", got)
	}
	first := time.Date(2026, 11, 1, 5, 20, 0, 0, time.UTC)
	if got := NextInLocation(s130, first, ny); !got.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Errorf("fall back first pass: got %v", got)
	}

	s230, _ := Parse("30 2 * * *")
	day := time.Date(2027, 3, 14, 0, 0, 0, 0, time.UTC)
	got = NextInLocation(s230, day, ny)
	if !got.Equal(time.Date(2027, 3, 14, 7, 30, 0, 0, time.UTC)) {
		t.Errorf("spring forward NY: got %v", got)
	}
	again := NextInLocation(s230, got, ny)
	if !again.Equal(time.Date(2027, 3, 15, 6, 30, 0, 0, time.UTC)) {
		t.Errorf("spring forward ran twice or skipped: %v", again)
	}

	// London: 01:30 does not exist on 2027-03-28, and 01:30 repeats on 2026-10-25.
	s130b, _ := Parse("30 1 * * *")
	got = NextInLocation(s130b, time.Date(2027, 3, 28, 0, 0, 0, 0, time.UTC), lon)
	if !got.Equal(time.Date(2027, 3, 28, 1, 30, 0, 0, time.UTC)) {
		t.Errorf("spring forward London: got %v", got)
	}
	got = NextInLocation(s130b, time.Date(2026, 10, 25, 0, 45, 0, 0, time.UTC), lon)
	if !got.Equal(time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)) && !got.After(time.Date(2026, 10, 25, 0, 45, 0, 0, time.UTC)) {
		t.Errorf("fall back London: got %v", got)
	}
	got = NextInLocation(s130b, time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC), lon)
	if !got.Equal(time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)) {
		t.Errorf("fall back London second pass: got %v", got)
	}

	for _, loc := range []*time.Location{time.UTC, fixed} {
		a := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		if got := NextInLocation(s130, a, loc); !got.Equal(time.Date(2026, 5, 2, 1, 30, 0, 0, time.UTC)) {
			t.Errorf("fixed zone: got %v", got)
		}
	}
}

func TestNextInLocation_AlwaysAfter(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	s, _ := Parse("*/10 0-4 * * *")
	at := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	for range 60 {
		n := NextInLocation(s, at, ny)
		if !n.After(at) {
			t.Fatalf("%v not after %v", n, at)
		}
		at = n
	}
}

func TestValidate(t *testing.T) {
	now := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	ny := mustLoc(t, "America/New_York")
	for _, ok := range []string{"0 3 * * *", "0 0 29 2 *", "@daily"} {
		if err := Validate(ok, ny, now); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	err := Validate("0 0 30 2 *", ny, now)
	if err == nil || !strings.Contains(err.Error(), "no 30th of February") {
		t.Errorf("feb 30: %v", err)
	}
	if err := Validate("0 0 31 4 *", nil, now); err == nil || !strings.Contains(err.Error(), "31st of April") {
		t.Errorf("apr 31: %v", err)
	}
	if Validate("1 2 3", nil, now) == nil {
		t.Error("garbage accepted")
	}
}

func TestDue(t *testing.T) {
	now := time.Now()
	if Due(time.Time{}, now) || Due(now.Add(time.Minute), now) || !Due(now, now) {
		t.Fatal("Due decision wrong")
	}
}
