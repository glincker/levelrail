package telemetry

import (
	"testing"
	"time"
)

func TestPresenceActiveWindow(t *testing.T) {
	clock := time.Unix(1000, 0)
	p := NewPresence(time.Minute)
	p.now = func() time.Time { return clock }
	if p.Active() {
		t.Fatal("Active() before any touch = true, want false")
	}
	p.Touch()
	clock = clock.Add(30 * time.Second)
	if !p.Active() {
		t.Fatal("Active() inside window = false, want true")
	}
	clock = clock.Add(31 * time.Second)
	if p.Active() {
		t.Fatal("Active() after window = true, want false")
	}
	var nilP *Presence
	nilP.Touch()
	if !nilP.Active() {
		t.Fatal("nil Presence must stay active")
	}
}

func TestCollectorDueAtSlowsWhenIdle(t *testing.T) {
	clock := time.Unix(1000, 0)
	p := NewPresence(time.Minute)
	p.now = func() time.Time { return clock }
	c := NewCollector(&fakeStatsSource{}, nil, 10*time.Second, nil)
	c.now = func() time.Time { return clock }
	c.SetPresence(p, 2*time.Minute)

	last := clock
	clock = clock.Add(10 * time.Second)
	if c.dueAt(last) {
		t.Fatal("idle tick 10s after last run is due, want skipped")
	}
	p.Touch()
	if !c.dueAt(last) {
		t.Fatal("tick with a viewer present skipped, want due")
	}
	clock = clock.Add(5 * time.Minute)
	if !c.dueAt(last) {
		t.Fatal("idle tick past idleInterval skipped, want due")
	}
}
