package application

import (
	"testing"
	"time"
)

type fakeSink struct {
	held     map[string]int
	released []string
}

func (f *fakeSink) Held(c string, n int, _ time.Time) { f.held[c] = n }
func (f *fakeSink) Released(c string)                 { f.released = append(f.released, c) }

func TestQuarantineSinkHeldRestoreAndRelease(t *testing.T) {
	sink := &fakeSink{held: map[string]int{}}
	b := NewRestartBackoff(time.Second, time.Second, time.Hour)
	b.SetBreaker(2, 0)
	b.SetSink(sink)
	now := time.Now()
	b.recordRestart("web-abcd1234", now)
	if len(sink.held) != 0 {
		t.Fatal("held before threshold")
	}
	b.recordRestart("web-abcd1234", now.Add(time.Second))
	b.recordRestart("web-abcd1234", now.Add(2*time.Second))
	if sink.held["web-abcd1234"] != 2 || len(sink.held) != 1 {
		t.Fatalf("held = %v, want one call with 2", sink.held)
	}
	fresh := NewRestartBackoff(time.Second, time.Second, time.Hour)
	fresh.SetBreaker(2, 0)
	fresh.Restore("web-abcd1234", 2, now)
	if ok, _ := fresh.Quarantined("web-abcd1234"); !ok {
		t.Fatal("restored container not quarantined")
	}
	fresh.SetSink(sink)
	if n := fresh.Resume("web"); n != 1 || len(sink.released) != 1 {
		t.Fatalf("resume n=%d released=%v", n, sink.released)
	}
}

func TestObserveDockerRestartsQuarantinesInsideWindow(t *testing.T) {
	sink := &fakeSink{held: map[string]int{}}
	b := NewRestartBackoff(time.Second, time.Second, time.Hour)
	b.SetBreaker(3, 0)
	b.SetDockerWindow(time.Minute)
	b.SetSink(sink)
	now := time.Now()
	name := "web-abcd1234"
	if b.ObserveDockerRestarts(name, 40, now) {
		t.Fatal("baseline must not count old restarts")
	}
	if b.ObserveDockerRestarts(name, 42, now.Add(10*time.Second)) {
		t.Fatal("two restarts is under the threshold")
	}
	if !b.ObserveDockerRestarts(name, 43, now.Add(20*time.Second)) {
		t.Fatal("third restart inside the window must quarantine")
	}
	if ok, n := b.Quarantined(name); !ok || n != 3 || sink.held[name] != 3 {
		t.Fatalf("quarantined=%v n=%d held=%v", ok, n, sink.held)
	}
	slow := "api-abcd1234"
	b.ObserveDockerRestarts(slow, 0, now)
	b.ObserveDockerRestarts(slow, 1, now.Add(2*time.Minute))
	b.ObserveDockerRestarts(slow, 2, now.Add(4*time.Minute))
	if b.ObserveDockerRestarts(slow, 3, now.Add(6*time.Minute)) {
		t.Fatal("restarts spread beyond the window must not quarantine")
	}
}

// lateSink releases the container from inside Held's first call, mimicking a Resume that
// lands between the breaker opening and the sink being told.
func TestHeldAfterResumeIsDropped(t *testing.T) {
	sink := &fakeSink{held: map[string]int{}}
	b := NewRestartBackoff(time.Second, time.Second, time.Hour)
	b.SetBreaker(1, 0)
	name := "web-abcd1234"
	b.mu.Lock()
	held, s := b.record(name, time.Now())
	b.mu.Unlock()
	b.SetSink(sink)
	b.Resume("web")
	b.notifyHeld(s, name, held, time.Now())
	b.notifyHeld(sink, name, 1, time.Now())
	if len(sink.held) != 0 {
		t.Fatalf("held after resume reached the sink: %v", sink.held)
	}
}

func TestRestoreSkippedWhenBreakerOff(t *testing.T) {
	b := NewRestartBackoff(time.Second, time.Second, time.Hour)
	b.Restore("web-abcd1234", 5, time.Now())
	if ok, _ := b.Quarantined("web-abcd1234"); ok {
		t.Fatal("restore must be ignored when the breaker is off")
	}
}

func TestObserveDockerRestartsLongGapRebaselines(t *testing.T) {
	b := NewRestartBackoff(time.Second, time.Second, time.Hour)
	b.SetBreaker(3, 0)
	b.SetDockerWindow(time.Minute)
	now := time.Now()
	b.ObserveDockerRestarts("web-abcd1234", 0, now)
	if b.ObserveDockerRestarts("web-abcd1234", 9, now.Add(time.Hour)) {
		t.Fatal("restarts seen across a gap longer than the window must not trip")
	}
	if !b.ObserveDockerRestarts("web-abcd1234", 12, now.Add(time.Hour+10*time.Second)) {
		t.Fatal("fresh restarts after re-baseline must still trip")
	}
}
