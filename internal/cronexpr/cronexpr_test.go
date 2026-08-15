package cronexpr

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, expr string) *Schedule {
	t.Helper()
	s, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", expr, err)
	}
	return s
}

func utc(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, time.UTC)
}

func TestParse_InvalidFieldCount(t *testing.T) {
	for _, expr := range []string{"", "* *", "* * * * * *", "0 3 * *"} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) expected an error, got nil", expr)
		}
	}
}

func TestParse_OutOfRangeAndMalformed(t *testing.T) {
	cases := []string{
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"* * 0 * *",   // day-of-month below minimum
		"* * * 13 *",  // month out of range
		"* * * * 8",   // day-of-week out of range (7 is the only accepted alias)
		"x * * * *",   // not a number
		"5-2 * * * *", // inverted range
		"*/0 * * * *", // zero step
	}
	for _, expr := range cases {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) expected an error, got nil", expr)
		}
	}
}

func TestParse_DowAlias7IsSunday(t *testing.T) {
	s := mustParse(t, "0 0 * * 7")
	// 2026-08-16 is a Sunday.
	if !s.dayMatches(utc(2026, 8, 16, 0, 0)) {
		t.Errorf("dow=7 should match Sunday")
	}
	if s.dayMatches(utc(2026, 8, 17, 0, 0)) {
		t.Errorf("dow=7 should not match Monday")
	}
}

func TestSchedule_Next_DailyAtThreeAM(t *testing.T) {
	s := mustParse(t, "0 3 * * *")

	got := s.Next(utc(2026, 8, 15, 12, 0)) // after 3am has already passed today
	want := utc(2026, 8, 16, 3, 0)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}

	got2 := s.Next(utc(2026, 8, 16, 2, 59))
	want2 := utc(2026, 8, 16, 3, 0)
	if !got2.Equal(want2) {
		t.Errorf("Next() = %v, want %v", got2, want2)
	}
}

func TestSchedule_Next_EveryFifteenMinutes(t *testing.T) {
	s := mustParse(t, "*/15 * * * *")

	got := s.Next(utc(2026, 8, 15, 12, 1))
	want := utc(2026, 8, 15, 12, 15)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}

	// Exactly on a matching minute: Next always returns strictly after t.
	got2 := s.Next(utc(2026, 8, 15, 12, 15))
	want2 := utc(2026, 8, 15, 12, 30)
	if !got2.Equal(want2) {
		t.Errorf("Next() = %v, want %v", got2, want2)
	}
}

func TestSchedule_Next_FirstOfMonth(t *testing.T) {
	s := mustParse(t, "0 0 1 * *")

	got := s.Next(utc(2026, 8, 15, 0, 0))
	want := utc(2026, 9, 1, 0, 0)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}

	// December rolls into the next year.
	got2 := s.Next(utc(2026, 12, 5, 0, 0))
	want2 := utc(2027, 1, 1, 0, 0)
	if !got2.Equal(want2) {
		t.Errorf("Next() = %v, want %v", got2, want2)
	}
}

func TestSchedule_Next_WeekdaysOnly(t *testing.T) {
	s := mustParse(t, "0 9 * * 1-5")

	// 2026-08-14 is a Friday; the next weekday 9am after it is Monday
	// 2026-08-17 (the intervening weekend is skipped).
	got := s.Next(utc(2026, 8, 14, 10, 0))
	want := utc(2026, 8, 17, 9, 0)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}
}

// TestSchedule_Next_DayOfMonthOrDayOfWeek exercises the classic cron
// disjunction: when both day-of-month and day-of-week are restricted,
// a date matches if EITHER matches, not both.
func TestSchedule_Next_DayOfMonthOrDayOfWeek(t *testing.T) {
	// "the 1st of the month, or any Friday" at midnight.
	s := mustParse(t, "0 0 1 * 5")

	// 2026-08-15 is a Saturday, not the 1st and not a Friday: should
	// skip forward to the next Friday, 2026-08-21, which comes before
	// the 1st of September.
	got := s.Next(utc(2026, 8, 15, 0, 0))
	want := utc(2026, 8, 21, 0, 0)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v (OR semantics: matches Friday even though day-of-month says 1)", got, want)
	}
}

func TestSchedule_Next_UnrestrictedDayOfWeekIsAND(t *testing.T) {
	// day-of-week left as "*" (unrestricted): only day-of-month applies,
	// ordinary AND, not the OR quirk.
	s := mustParse(t, "0 0 15 * *")

	got := s.Next(utc(2026, 8, 1, 0, 0))
	want := utc(2026, 8, 15, 0, 0)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}
}

func TestSchedule_Next_ConvertsToUTC(t *testing.T) {
	s := mustParse(t, "0 3 * * *")
	loc := time.FixedZone("UTC-5", -5*60*60)
	// 2026-08-15 23:00 in UTC-5 is 2026-08-16 04:00 UTC, already past
	// 3am UTC that day.
	local := time.Date(2026, 8, 15, 23, 0, 0, 0, loc)

	got := s.Next(local)
	want := utc(2026, 8, 17, 3, 0)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}
}

func TestSchedule_Next_ListOfMinutes(t *testing.T) {
	s := mustParse(t, "5,35 * * * *")

	got := s.Next(utc(2026, 8, 15, 12, 10))
	want := utc(2026, 8, 15, 12, 35)
	if !got.Equal(want) {
		t.Errorf("Next() = %v, want %v", got, want)
	}
}
