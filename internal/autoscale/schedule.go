package autoscale

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// maxSchedules bounds the windows one policy may carry.
const maxSchedules = 16

var dayIndex = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Schedule raises the effective minimum replicas while its window is open.
// Days are three letter names (mon..sun), empty meaning every day. A window
// whose End is before its Start runs overnight and belongs to its Start day.
type Schedule struct {
	Days        []string `json:"days,omitempty"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Timezone    string   `json:"timezone,omitempty"`
	MinReplicas int      `json:"min_replicas"`
}

func parseClock(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a time of day like 08:00", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func (s Schedule) location() (*time.Location, error) {
	if s.Timezone == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(s.Timezone)
}

func (s Schedule) dayOn(d time.Weekday) bool {
	if len(s.Days) == 0 {
		return true
	}
	for _, name := range s.Days {
		if dayIndex[strings.ToLower(name)] == d {
			return true
		}
	}
	return false
}

// Validate rejects a window that could not be evaluated.
func (s Schedule) Validate() error {
	start, err := parseClock(s.Start)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	end, err := parseClock(s.End)
	if err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if start == end {
		return errors.New("start and end must differ")
	}
	for _, d := range s.Days {
		if _, ok := dayIndex[strings.ToLower(d)]; !ok {
			return fmt.Errorf("days: %q is not one of mon, tue, wed, thu, fri, sat, sun", d)
		}
	}
	if _, err := s.location(); err != nil {
		return fmt.Errorf("timezone: %q is not a known time zone", s.Timezone)
	}
	if s.MinReplicas < 1 {
		return errors.New("min_replicas must be at least 1")
	}
	return nil
}

// Active reports whether now falls inside the window. An invalid window is never active.
func (s Schedule) Active(now time.Time) bool {
	loc, err := s.location()
	if err != nil {
		return false
	}
	start, err1 := parseClock(s.Start)
	end, err2 := parseClock(s.End)
	if err1 != nil || err2 != nil || start == end {
		return false
	}
	local := now.In(loc)
	m := local.Hour()*60 + local.Minute()
	today := local.Weekday()
	if start < end {
		return s.dayOn(today) && m >= start && m < end
	}
	yesterday := (today + 6) % 7
	return (s.dayOn(today) && m >= start) || (s.dayOn(yesterday) && m < end)
}

// EffectiveMin is the floor at now: the base minimum, raised by any open window.
func (p Policy) EffectiveMin(now time.Time) int {
	floor := p.MinReplicas
	for _, s := range p.Schedules {
		if s.MinReplicas > floor && s.Active(now) {
			floor = s.MinReplicas
		}
	}
	return floor
}

func (p Policy) validateSchedules() error {
	if len(p.Schedules) > maxSchedules {
		return fmt.Errorf("schedules: at most %d windows", maxSchedules)
	}
	for i, s := range p.Schedules {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("schedules[%d]: %w", i, err)
		}
		if s.MinReplicas > p.MaxReplicas {
			return fmt.Errorf("schedules[%d]: min_replicas must not exceed max_replicas", i)
		}
	}
	return nil
}
