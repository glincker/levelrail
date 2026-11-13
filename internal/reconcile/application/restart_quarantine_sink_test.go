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
	fresh.Restore("web-abcd1234", 2, now)
	if ok, _ := fresh.Quarantined("web-abcd1234"); !ok {
		t.Fatal("restored container not quarantined")
	}
	fresh.SetSink(sink)
	if n := fresh.Resume("web"); n != 1 || len(sink.released) != 1 {
		t.Fatalf("resume n=%d released=%v", n, sink.released)
	}
}
