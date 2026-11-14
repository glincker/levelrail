package api

import (
	"testing"
	"time"
)

func TestErrorFirstSeenDedup(t *testing.T) {
	var s errorTrackingState
	now := time.Now()
	if !s.firstSeen("a", "e1", now, time.Hour, 10) || s.firstSeen("a", "e1", now.Add(time.Minute), time.Hour, 10) {
		t.Fatal("same id inside window must be dropped once")
	}
	if !s.firstSeen("b", "e1", now, time.Hour, 10) || !s.firstSeen("a", "e1", now.Add(2*time.Hour), time.Hour, 10) {
		t.Fatal("other app or expired window must pass")
	}
}
