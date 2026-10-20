package main

import (
	"testing"
	"time"
)

func TestComputeRolloutOutcome_QueuedAndCanceled(t *testing.T) {
	done := time.Now()
	tests := []struct {
		status string
		want   string
	}{
		{"queued", "pending"},
		{"canceled", "failed"},
		{"superseded", "failed"},
	}
	for _, tt := range tests {
		got := computeRolloutOutcome(deployAttemptResource{Status: tt.status, FinishedAt: &done}, nil)
		if got.state != tt.want {
			t.Errorf("%s: state = %q, want %q", tt.status, got.state, tt.want)
		}
	}
}
