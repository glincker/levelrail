package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestComputeDeployOutcome_RunToCompletion(t *testing.T) {
	finished := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	after := finished.Add(time.Minute)
	a := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, FinishedAt: &finished}
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{"long-running service deployed", "Deployed", OutcomeHealthy},
		{"one-shot completed", application.ReasonCompleted, OutcomeHealthy},
		{"one-shot exited nonzero", "ExitedDuringReadiness", OutcomeFailed},
		{"one-shot still running", application.ReasonRunningToComplete, OutcomeInProgress},
		{"one-shot completion timeout", application.ReasonCompletionTimeout, OutcomeInProgress},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conds := []reconcile.Condition{{Reason: tc.reason, LastTransitionTime: after}}
			if got := computeDeployOutcome(a, true, conds); got != tc.want {
				t.Errorf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}
