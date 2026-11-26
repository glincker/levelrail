package database

import (
	"testing"
	"time"
)

func TestCrashLoopBackoffGrows(t *testing.T) {
	s := NewSharedState()
	now := time.Unix(1000, 0)
	var waits []time.Duration
	for i := 0; i < 5; i++ {
		_, w := s.noteCrash("db", now, time.Minute, 3)
		waits = append(waits, w)
		now = now.Add(w + time.Second)
	}
	if waits[0] != 0 || waits[1] != 0 || waits[2] != crashBackoffBase || waits[3] != 2*crashBackoffBase {
		t.Fatalf("waits = %v", waits)
	}
}

func TestCrashLoopHoldsDuringBackoff(t *testing.T) {
	s := NewSharedState()
	now := time.Unix(1000, 0)
	for i := 0; i < 3; i++ {
		s.noteCrash("db", now, time.Minute, 3)
	}
	if _, w := s.noteCrash("db", now.Add(time.Second), time.Minute, 3); w <= 0 {
		t.Fatalf("expected hold, got %v", w)
	}
}
