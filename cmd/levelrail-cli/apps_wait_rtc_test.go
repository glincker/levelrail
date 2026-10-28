package main

import (
	"testing"
	"time"
)

func TestComputeRolloutOutcome_RunToCompletion(t *testing.T) {
	fin := time.Now()
	a := deployAttemptResource{Status: "succeeded", FinishedAt: &fin}
	tests := []struct {
		reason string
		want   string
	}{
		{"Completed", "succeeded"},
		{"ExitedDuringReadiness", "failed"},
		{"RunningToCompletion", "pending"},
		{"CompletionTimeout", "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			conds := []conditionResource{{Reason: tt.reason, LastTransitionTime: fin.Add(time.Second)}}
			if got := computeRolloutOutcome(a, conds); got.state != tt.want {
				t.Errorf("state = %q, want %q", got.state, tt.want)
			}
		})
	}
}

func TestComputeRolloutOutcome_CompletionTimeoutThenCompleted(t *testing.T) {
	fin := time.Now()
	a := deployAttemptResource{Status: "succeeded", FinishedAt: &fin}
	timedOut := conditionResource{Reason: "CompletionTimeout", LastTransitionTime: fin.Add(time.Minute)}
	done := conditionResource{Reason: "Completed", LastTransitionTime: fin.Add(2 * time.Minute)}
	if got := computeRolloutOutcome(a, []conditionResource{timedOut, done}); got.state != "succeeded" {
		t.Fatalf("state after a later Completed = %q, want succeeded", got.state)
	}
}
