// Package nodeinsight holds the pure logic behind the node health timeline,
// the agent doctor extras and the agent version skew report, so the API, the
// CLI and the dashboard all render one definition.
package nodeinsight

import (
	"sort"
	"time"
)

// Event is one recorded status transition.
type Event struct {
	From, To string
	At       time.Time
}

// Segment is a stretch of time a node spent in one state.
type Segment struct {
	State   string    `json:"state"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Seconds int64     `json:"seconds"`
}

// Timeline is a node's state over a window. UptimePercent is nil when the
// node was never online or offline inside the window (for example pending).
type Timeline struct {
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	Segments      []Segment `json:"segments"`
	Transitions   int       `json:"transitions"`
	UptimePercent *float64  `json:"uptime_percent,omitempty"`
}

// BuildTimeline turns transitions into contiguous segments over [now-window, now].
// current is the node's state now and fills the span when there are no events.
func BuildTimeline(events []Event, current string, window time.Duration, now time.Time) Timeline {
	start := now.Add(-window)
	asc := append([]Event(nil), events...)
	sort.Slice(asc, func(i, j int) bool { return asc[i].At.Before(asc[j].At) })

	state := current
	var inside []Event
	for _, e := range asc {
		if e.At.After(now) {
			continue
		}
		if e.At.After(start) {
			inside = append(inside, e)
		}
	}
	if len(inside) > 0 {
		state = inside[0].From
	}

	tl := Timeline{From: start, To: now, Segments: []Segment{}, Transitions: len(inside)}
	cursor := start
	add := func(s string, to time.Time) {
		if !to.After(cursor) {
			return
		}
		if n := len(tl.Segments); n > 0 && tl.Segments[n-1].State == s {
			tl.Segments[n-1].To = to
			tl.Segments[n-1].Seconds = int64(to.Sub(tl.Segments[n-1].From).Seconds())
		} else {
			tl.Segments = append(tl.Segments, Segment{State: s, From: cursor, To: to, Seconds: int64(to.Sub(cursor).Seconds())})
		}
		cursor = to
	}
	for _, e := range inside {
		add(state, e.At)
		state = e.To
	}
	add(state, now)

	var up, tracked int64
	for _, s := range tl.Segments {
		switch s.State {
		case "online":
			up += s.Seconds
			tracked += s.Seconds
		case "offline":
			tracked += s.Seconds
		}
	}
	if tracked > 0 {
		pct := float64(up) * 100 / float64(tracked)
		pct = float64(int(pct*100+0.5)) / 100
		tl.UptimePercent = &pct
	}
	return tl
}
