// Package cronexpr parses standard 5-field cron expressions ("minute hour
// day-of-month month day-of-week") and computes the next matching time, in UTC
// or in a given time zone. It only evaluates schedules; it does not run
// anything.
package cronexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// field bounds for the five standard cron positions.
const (
	minuteMin, minuteMax = 0, 59
	hourMin, hourMax     = 0, 23
	domMin, domMax       = 1, 31
	monthMin, monthMax   = 1, 12
	dowMin, dowMax       = 0, 6 // 0 = Sunday, matching time.Weekday
)

// fieldSet is a bitmask over one cron field's legal value range. uint64
// covers every field here (minute needs the most bits, 60, well under
// 64), so one representation serves all five without a slice.
type fieldSet uint64

func (fs fieldSet) has(v int) bool { return fs&(1<<uint(v)) != 0 }

// Schedule is a parsed, immutable 5-field cron expression, safe for
// concurrent use by multiple goroutines (every method only reads its own
// fields, computed once at Parse time).
type Schedule struct {
	minute, hour, dom, month, dow fieldSet
	// domRestricted/dowRestricted record whether the *source expression*
	// narrowed dom/dow below "every value in range" (i.e. was not
	// literally "*"), which is what standard cron's day-matching quirk
	// keys off, not what the resulting bitmask happens to contain: a
	// dom field spelled "1-31" is indistinguishable in bitmask form from
	// "*", but cron treats the two differently for this one rule (a
	// hand-written "1-31" is a deliberate restriction and should not
	// silently combine with dow, "*" is not a restriction at all), so
	// these are tracked as their own booleans rather than derived from
	// the bitmask after the fact.
	domRestricted, dowRestricted bool
}

// macros are the standard cron shortcuts, expanded before field parsing.
var macros = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

func expandMacro(expr string) string {
	if full, ok := macros[strings.ToLower(strings.TrimSpace(expr))]; ok {
		return full
	}
	return expr
}

// Parse parses a standard 5-field cron expression. Fields are separated
// by whitespace; each field is one of "*", a single integer, an
// "a-b" range, or any of those with a "/step" suffix (e.g. "*/15",
// "10-20/5"), optionally comma-separating multiple such terms in one
// field (e.g. "1,15,30"). day-of-week accepts 7 as an alias for 0
// (Sunday), the common convention several real cron implementations
// also accept, though this parser does not accept month or day names
// (JAN, MON, ...): app.yaml's own schedule field is documented purely as
// a cron expression, and every worked example in this codebase uses
// numeric fields only, so name support would be untested surface with no
// current caller.
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(expandMacro(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cronexpr: expected 5 fields (minute hour day-of-month month day-of-week), got %d in %q", len(fields), expr)
	}

	minute, err := parseField(fields[0], minuteMin, minuteMax, false)
	if err != nil {
		return nil, fmt.Errorf("cronexpr: minute field: %w", err)
	}
	hour, err := parseField(fields[1], hourMin, hourMax, false)
	if err != nil {
		return nil, fmt.Errorf("cronexpr: hour field: %w", err)
	}
	dom, err := parseField(fields[2], domMin, domMax, false)
	if err != nil {
		return nil, fmt.Errorf("cronexpr: day-of-month field: %w", err)
	}
	month, err := parseField(fields[3], monthMin, monthMax, false)
	if err != nil {
		return nil, fmt.Errorf("cronexpr: month field: %w", err)
	}
	dow, err := parseField(fields[4], dowMin, dowMax, true)
	if err != nil {
		return nil, fmt.Errorf("cronexpr: day-of-week field: %w", err)
	}

	return &Schedule{
		minute: minute, hour: hour, dom: dom, month: month, dow: dow,
		domRestricted: fields[2] != "*",
		dowRestricted: fields[4] != "*",
	}, nil
}

// parseField parses one comma-separated cron field into a fieldSet.
// dowAlias7 enables the day-of-week-only "7 means Sunday (0)" alias.
func parseField(field string, fieldMin, fieldMax int, dowAlias7 bool) (fieldSet, error) {
	var fs fieldSet
	for _, term := range strings.Split(field, ",") {
		if term == "" {
			return 0, fmt.Errorf("empty term in %q", field)
		}

		base, step := term, 1
		if idx := strings.IndexByte(term, '/'); idx >= 0 {
			base = term[:idx]
			s, err := strconv.Atoi(term[idx+1:])
			if err != nil || s <= 0 {
				return 0, fmt.Errorf("invalid step in %q", term)
			}
			step = s
		}

		lo, hi, err := parseFieldTerm(term, base, fieldMin, fieldMax, step, dowAlias7)
		if err != nil {
			return 0, err
		}

		if lo < fieldMin || hi > fieldMax || lo > hi {
			return 0, fmt.Errorf("value out of range [%d,%d] in %q", fieldMin, fieldMax, term)
		}
		for v := lo; v <= hi; v += step {
			fs |= 1 << uint(v)
		}
	}
	return fs, nil
}

// parseFieldTerm resolves one term's base value (the part before an
// optional "/step") into an inclusive [lo, hi] range, split out of
// parseField's own loop purely to keep each function's cognitive
// complexity low: this is the "*" / "a-b" / "N" three-way switch, the
// step-open-ended-range and dowAlias7 rules stay here since both only
// apply once base has resolved to a single value.
func parseFieldTerm(term, base string, fieldMin, fieldMax, step int, dowAlias7 bool) (int, int, error) {
	switch {
	case base == "*":
		return fieldMin, fieldMax, nil
	case strings.Contains(base, "-"):
		bounds := strings.SplitN(base, "-", 2)
		lo, err := strconv.Atoi(bounds[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range start in %q", term)
		}
		hi, err := strconv.Atoi(bounds[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range end in %q", term)
		}
		return lo, hi, nil
	default:
		v, err := strconv.Atoi(base)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid value %q", base)
		}
		if dowAlias7 && v == 7 {
			v = 0
		}
		hi := v
		// A bare "N/step" (no explicit range) means "N, N+step, ... up
		// to the field's max", the same convention every standard cron
		// implementation applies: the step turns a single value into
		// an open-ended range, it doesn't stay a single value.
		if step != 1 {
			hi = fieldMax
		}
		return v, hi, nil
	}
}

// searchYears bounds how far Next looks ahead. Eight years plus a day
// spans the longest gap between two Feb 29 dates (2096 to 2104, across the
// non-leap 2100).
const searchYears = 8

// maxDaysInMonth is the longest each month can be (Feb counts its leap day).
var maxDaysInMonth = [13]int{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// impossible reports a schedule whose only day restriction is a day-of-month
// that no selected month ever has, so Next can answer without scanning.
func (s *Schedule) impossible() bool {
	if !s.domRestricted || s.dowRestricted {
		return false
	}
	for m := monthMin; m <= monthMax; m++ {
		if !s.month.has(m) {
			continue
		}
		for d := domMin; d <= maxDaysInMonth[m]; d++ {
			if s.dom.has(d) {
				return false
			}
		}
	}
	return true
}

// Next returns the earliest minute-aligned UTC time strictly after t
// that this Schedule matches, or the zero time.Time if no match exists
// within the search horizon. Cron fields have no timezone of their own, so
// t is converted to UTC here; NextInLocation handles other zones.
func (s *Schedule) Next(t time.Time) time.Time {
	if s.impossible() {
		return time.Time{}
	}
	t = t.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(searchYears, 0, 1)

	for t.Before(limit) {
		if !s.month.has(int(t.Month())) {
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
			continue
		}
		if !s.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
			continue
		}
		if !s.hour.has(t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC).Add(time.Hour)
			continue
		}
		if !s.minute.has(t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

// NextInLocation returns the next time schedule matches strictly after
// after, evaluated on loc's wall clock, or the zero time when there is none.
// In a repeated DST hour it picks the first instant after "after"; a wall time
// inside a spring-forward gap runs once, shifted past the gap.
func NextInLocation(schedule *Schedule, after time.Time, loc *time.Location) time.Time {
	local := after.In(loc)
	wall := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
	for range 8 {
		nextWall := schedule.Next(wall)
		if nextWall.IsZero() {
			return time.Time{}
		}
		if inst, ok := resolveWall(nextWall, loc, after); ok {
			return inst.In(loc)
		}
		wall = nextWall
	}
	return time.Time{}
}

// resolveWall maps a wall clock (carried in UTC fields) to the earliest real
// instant in loc strictly after "after".
func resolveWall(wall time.Time, loc *time.Location, after time.Time) (time.Time, bool) {
	offsets := map[int]struct{}{}
	for _, probe := range []time.Duration{-24 * time.Hour, 0, 24 * time.Hour} {
		_, off := wall.Add(probe).In(loc).Zone()
		offsets[off] = struct{}{}
	}
	var best time.Time
	for off := range offsets {
		inst := wall.Add(-time.Duration(off) * time.Second)
		w := inst.In(loc)
		if w.Hour() != wall.Hour() || w.Minute() != wall.Minute() || w.Day() != wall.Day() {
			continue
		}
		if inst.After(after) && (best.IsZero() || inst.Before(best)) {
			best = inst
		}
	}
	if !best.IsZero() {
		return best, true
	}
	// Spring-forward gap: no offset yields this wall time, so apply the
	// pre-gap offset once, which lands the same minute after the gap.
	_, before := wall.Add(-24 * time.Hour).In(loc).Zone()
	inst := wall.Add(-time.Duration(before) * time.Second)
	if inst.After(after) && gapWall(wall, loc) {
		return inst, true
	}
	return time.Time{}, false
}

// gapWall reports that no instant in loc displays wall.
func gapWall(wall time.Time, loc *time.Location) bool {
	for _, d := range []time.Duration{-24 * time.Hour, 0, 24 * time.Hour} {
		_, off := wall.Add(d).In(loc).Zone()
		w := wall.Add(-time.Duration(off) * time.Second).In(loc)
		if w.Hour() == wall.Hour() && w.Minute() == wall.Minute() && w.Day() == wall.Day() {
			return false
		}
	}
	return true
}

// Due reports whether a schedule's next run has arrived; an unknown (zero)
// next run is never due, so a schedule that cannot fire cannot hot loop.
func Due(next, now time.Time) bool {
	return !next.IsZero() && !next.After(now)
}

// Validate parses expr and fails when it has no occurrence from now within
// the search horizon, so an impossible date is rejected at save time.
func Validate(expr string, loc *time.Location, now time.Time) error {
	sched, err := Parse(expr)
	if err != nil {
		return err
	}
	if loc == nil {
		loc = time.UTC
	}
	if NextInLocation(sched, now, loc).IsZero() {
		return fmt.Errorf("cronexpr: schedule never runs: %s", sched.neverReason())
	}
	return nil
}

// neverReason names the missing date for the common impossible shapes.
func (s *Schedule) neverReason() string {
	if s.domRestricted && !s.dowRestricted {
		for d := domMin; d <= domMax; d++ {
			if !s.dom.has(d) {
				continue
			}
			for m := monthMin; m <= monthMax; m++ {
				if s.month.has(m) {
					return fmt.Sprintf("no %d%s of %s", d, ordinalSuffix(d), time.Month(m))
				}
			}
		}
	}
	return "no matching date within 8 years"
}

func ordinalSuffix(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// dayMatches applies the standard cron day-of-month/day-of-week
// disjunction: when only one of the two fields is a real restriction
// (the other is "*"), only the restricted one has to match, the ordinary
// AND behavior every other field pair uses. When *both* are restricted,
// cron switches to OR (either matching is enough), a deliberately
// surprising rule every real cron implementation (Vixie cron, systemd
// timers translated from cron syntax, POSIX cron) shares, not a bug: it
// is what makes "the 1st of the month, or any Friday" expressible at
// all as a single field pair, since AND would make that combination
// (and most others) unsatisfiable.
func (s *Schedule) dayMatches(t time.Time) bool {
	domOK := s.dom.has(t.Day())
	dowOK := s.dow.has(int(t.Weekday()))
	if s.domRestricted && s.dowRestricted {
		return domOK || dowOK
	}
	return domOK && dowOK
}
