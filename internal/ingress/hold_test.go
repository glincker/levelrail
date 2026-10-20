package ingress

import (
	"testing"
	"time"
)

func TestHoldTracker(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		window time.Duration
		route  bool
		at     time.Duration
		want   bool
	}{
		{"within window", time.Minute, true, 30 * time.Second, true},
		{"expired", time.Minute, true, 2 * time.Minute, false},
		{"never routed", time.Minute, false, 0, false},
		{"disabled", 0, true, time.Second, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := NewHoldTracker(tt.window)
			if tt.route {
				tr.Routed("a.test", now)
			}
			if got := tr.Held("a.test", now.Add(tt.at)); got != tt.want {
				t.Errorf("Held = %v, want %v", got, tt.want)
			}
		})
	}
	var nilTracker *HoldTracker
	nilTracker.Routed("x", now)
	if nilTracker.Held("x", now) {
		t.Error("nil tracker must never hold")
	}
}
