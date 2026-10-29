package main

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestComputeRolloutOutcome_ServerOutcomeWins(t *testing.T) {
	finished := time.Now()
	a := deployAttemptResource{Status: "succeeded", FinishedAt: &finished, Outcome: "failed", OutcomeReason: "web keeps exiting (3 restarts)"}
	got := computeRolloutOutcome(a, nil)
	if got.state != "failed" || got.reason != a.OutcomeReason {
		t.Errorf("outcome = %+v, want failed with the server reason", got)
	}
	a.Outcome = "in_progress"
	if got := computeRolloutOutcome(a, nil); got.state != "pending" {
		t.Errorf("in_progress = %+v, want pending", got)
	}
}

func TestStatusOutcomeLine(t *testing.T) {
	failed := apiclient.AppStatus{Outcome: "failed", OutcomeReason: "web keeps exiting"}
	if got := statusOutcomeLine(failed, nil); got != "latest deploy failed: web keeps exiting" {
		t.Errorf("line = %q", got)
	}
	if got := statusOutcomeLine(apiclient.AppStatus{Outcome: "healthy", OutcomeReason: "serving"}, nil); got != "" {
		t.Errorf("healthy line = %q, want empty", got)
	}
	crash := []conditionResource{{Reason: "CrashLoopBackOff"}}
	if got := statusOutcomeLine(apiclient.AppStatus{}, crash); got == "" {
		t.Error("older server should keep the local crash-loop line")
	}
}
