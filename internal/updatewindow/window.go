// Package updatewindow parses the daily maintenance window used by agent
// updates. It has no internal dependencies so settings validation can use it.
package updatewindow

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const minutesPerDay = 24 * 60

// Window is a daily local time range during which automatic updates may run.
// The zero value means any time.
type Window struct {
	set        bool
	start, end int
}

// Parse reads "HH:MM-HH:MM". Empty means any time; a range whose end is
// before its start wraps past midnight.
func Parse(s string) (Window, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Window{}, nil
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return Window{}, fmt.Errorf("window %q must look like 02:00-05:00", s)
	}
	start, err := parseClock(a)
	if err != nil {
		return Window{}, err
	}
	end, err := parseClock(b)
	if err != nil {
		return Window{}, err
	}
	if start == end {
		return Window{}, fmt.Errorf("window %q has the same start and end; leave it empty for any time", s)
	}
	return Window{set: true, start: start, end: end}, nil
}

func parseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("time %q must be HH:MM between 00:00 and 23:59", s)
	}
	return hh*60 + mm, nil
}

// Open reports whether now (in its own location) is inside the window.
func (w Window) Open(now time.Time) bool {
	if !w.set {
		return true
	}
	m := now.Hour()*60 + now.Minute()
	if w.start < w.end {
		return m >= w.start && m < w.end
	}
	return m >= w.start || m < w.end
}
