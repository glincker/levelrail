// Package cronexpr parses and evaluates standard 5-field cron
// expressions ("minute hour day-of-month month day-of-week", e.g.
// "0 3 * * *"), the exact format internal/spec.Backup.Schedule and
// app.yaml's own databases[].backup.schedule field document (see the
// worked example in the repo plan's section 4.9).
//
// This is a small, purpose-built parser, not a vendored dependency:
// go.mod carries no cron library today, and standard 5-field cron
// (literals, "*", comma-separated lists, "a-b" ranges, and "/step"
// steps, plus the classic "day matches if EITHER day-of-month OR
// day-of-week matches, when both are restricted" quirk every real cron
// implementation shares) is a small, well-understood grammar that fits
// comfortably in one file with table-driven tests, matching this
// project's own stated preference for a handful of well-tested internal
// primitives over pulling in a framework for something this narrow (repo
// plan section 4.1's "do not pull in a heavy framework" rule, applied
// here one layer down from the HTTP framework it was written about).
// Nothing here schedules anything or runs a goroutine: that is
// internal/backup.Scheduler's job, built on top of the Next method this
// package exposes, the same "parser is a pure value, the loop lives
// elsewhere" split internal/spec keeps between Parse and every package
// that actually acts on a Spec.
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
	fields := strings.Fields(expr)
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

		lo, hi := fieldMin, fieldMax
		switch {
		case base == "*":
			// lo, hi already cover the field's full range.
		case strings.Contains(base, "-"):
			bounds := strings.SplitN(base, "-", 2)
			var err error
			lo, err = strconv.Atoi(bounds[0])
			if err != nil {
				return 0, fmt.Errorf("invalid range start in %q", term)
			}
			hi, err = strconv.Atoi(bounds[1])
			if err != nil {
				return 0, fmt.Errorf("invalid range end in %q", term)
			}
		default:
			v, err := strconv.Atoi(base)
			if err != nil {
				return 0, fmt.Errorf("invalid value %q", base)
			}
			if dowAlias7 && v == 7 {
				v = 0
			}
			lo, hi = v, v
			// A bare "N/step" (no explicit range) means "N, N+step,
			// ... up to the field's max", the same convention every
			// standard cron implementation applies: the step turns a
			// single value into an open-ended range, it doesn't stay a
			// single value.
			if step != 1 {
				hi = fieldMax
			}
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

// searchHorizon bounds how far into the future Next searches before
// giving up. Four years covers every real calendar quirk (leap years,
// "31st" in months that don't have one) without risking an unbounded
// loop for a schedule that can genuinely never match (e.g. day-of-month
// 31 combined with a month field that excludes every 31-day month, were
// day-of-month ever the sole restriction, which it never actually is
// alone thanks to the day-matching OR rule, but the bound stays here as
// a hard backstop regardless of which combination a caller writes).
const searchHorizon = 4 * 365 * 24 * time.Hour

// Next returns the earliest minute-aligned UTC time strictly after t
// that this Schedule matches, or the zero time.Time if no match exists
// within searchHorizon. Cron fields have no timezone concept of their
// own; every caller in this codebase (internal/backup.Scheduler) works
// in UTC exclusively, the same "everything server-side is UTC, a client
// converts for display" convention store.BackupHistory's own
// RFC3339-in-UTC timestamps already establish, so t is converted to UTC
// here rather than accepting an implicit local-time assumption.
func (s *Schedule) Next(t time.Time) time.Time {
	t = t.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(searchHorizon)

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
