package probe

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultExpectedStatus is the status set a probe with no expected_status accepts.
const DefaultExpectedStatus = "200-299"

type statusRange struct{ lo, hi int }

// StatusSet is a parsed expected_status value: a union of status codes and ranges.
type StatusSet struct {
	ranges []statusRange
	raw    string
}

// ParseStatusSet parses "200-399", "200,204,301-302" or a single code.
// Commas and whitespace both separate entries; an empty string yields the 2xx default.
func ParseStatusSet(s string) (StatusSet, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = DefaultExpectedStatus
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return StatusSet{}, fmt.Errorf("expected_status %q: no status codes given", s)
	}
	set := StatusSet{raw: strings.Join(fields, ",")}
	for _, f := range fields {
		r, err := parseStatusRange(f)
		if err != nil {
			return StatusSet{}, fmt.Errorf("expected_status %q: %w", s, err)
		}
		set.ranges = append(set.ranges, r)
	}
	return set, nil
}

func parseStatusRange(f string) (statusRange, error) {
	loStr, hiStr, isRange := strings.Cut(f, "-")
	lo, err := parseStatusCode(loStr)
	if err != nil {
		return statusRange{}, err
	}
	if !isRange {
		return statusRange{lo: lo, hi: lo}, nil
	}
	hi, err := parseStatusCode(hiStr)
	if err != nil {
		return statusRange{}, err
	}
	if lo > hi {
		return statusRange{}, fmt.Errorf("range %q runs backwards, write it low-high", f)
	}
	return statusRange{lo: lo, hi: hi}, nil
}

func parseStatusCode(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 100 || n > 599 {
		return 0, fmt.Errorf("%q is not an HTTP status code between 100 and 599 (use a code like 204 or a range like 200-399)", s)
	}
	return n, nil
}

// Contains reports whether code is in the set.
func (s StatusSet) Contains(code int) bool {
	for _, r := range s.ranges {
		if code >= r.lo && code <= r.hi {
			return true
		}
	}
	return false
}

// String returns the normalized comma-separated form.
func (s StatusSet) String() string { return s.raw }
